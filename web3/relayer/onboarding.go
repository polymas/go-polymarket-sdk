package relayer

import (
	"context"
	"fmt"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

// DepositWalletOnboardingStage is the next stable checkpoint in onboarding.
type DepositWalletOnboardingStage string

const (
	OnboardingStageFundingRequired  DepositWalletOnboardingStage = "funding_required"
	OnboardingStageApprovalsPending DepositWalletOnboardingStage = "approvals_pending"
	OnboardingStageReady            DepositWalletOnboardingStage = "ready"
)

// DepositWalletOnboardingResult describes what was completed and what remains.
// A funding_required result is not an error: transfer pUSD or USDC.e to
// Account.Wallet, then call OnboardDepositWallet again.
type DepositWalletOnboardingResult struct {
	Account         web3.WalletAccount
	Stage           DepositWalletOnboardingStage
	Ready           bool
	MinimumPUSD     float64
	PUSDBalance     float64
	USDCEBalance    float64
	Missing         []string
	DeployReceipt   *types.TransactionReceipt
	ApprovalReceipt *types.TransactionReceipt
}

type depositWalletOnboardingOps interface {
	resolve(context.Context) (*web3.WalletAccount, error)
	deploy(context.Context) (*types.TransactionReceipt, error)
	state(context.Context, common.Address) (*v2State, *big.Int, error)
	ensure(context.Context) (*types.TransactionReceipt, error)
}

type gaslessDepositWalletOnboardingOps struct{ client *GaslessClient }

func (ops gaslessDepositWalletOnboardingOps) resolve(ctx context.Context) (*web3.WalletAccount, error) {
	return ops.client.ResolveWalletAccount(ctx)
}

func (ops gaslessDepositWalletOnboardingOps) deploy(ctx context.Context) (*types.TransactionReceipt, error) {
	return ops.client.DeployDepositWalletContext(ctx, false)
}

func (ops gaslessDepositWalletOnboardingOps) state(ctx context.Context, wallet common.Address) (*v2State, *big.Int, error) {
	state, err := ops.client.readV2State(ctx, wallet)
	if err != nil {
		return nil, nil, err
	}
	pusd, err := ops.client.callERC20Uint(ctx, common.HexToAddress(internal.PolygonPUSD), "balanceOf", wallet)
	if err != nil {
		return nil, nil, fmt.Errorf("pUSD.balanceOf: %w", err)
	}
	return state, pusd, nil
}

func (ops gaslessDepositWalletOnboardingOps) ensure(ctx context.Context) (*types.TransactionReceipt, error) {
	return ops.client.EnsureV2ReadyContext(ctx)
}

// OnboardDepositWallet runs an idempotent deploy → funding checkpoint →
// approvals → ready-check workflow for a Deposit Wallet. minimumPUSD is the
// minimum trading collateral required by the business; zero checks deployment
// and approvals without requiring a positive balance.
//
// The SDK intentionally does not transfer funds from the controlling EOA.
// When funds are missing it returns funding_required with the exact destination
// wallet; once funded, repeating this method resumes from on-chain state.
func (c *GaslessClient) OnboardDepositWallet(ctx context.Context, minimumPUSD float64) (*DepositWalletOnboardingResult, error) {
	return runDepositWalletOnboarding(ctx, minimumPUSD, gaslessDepositWalletOnboardingOps{client: c})
}

func runDepositWalletOnboarding(ctx context.Context, minimumPUSD float64, ops depositWalletOnboardingOps) (*DepositWalletOnboardingResult, error) {
	if math.IsNaN(minimumPUSD) || math.IsInf(minimumPUSD, 0) || minimumPUSD < 0 {
		return nil, fmt.Errorf("minimumPUSD must be a finite non-negative value")
	}
	account, err := ops.resolve(ctx)
	if err != nil {
		return nil, err
	}
	if account.WalletType != web3.WalletTypeDepositWallet || account.SignatureType != types.DepositWalletSignatureType {
		return nil, fmt.Errorf("OnboardDepositWallet requires DEPOSIT_WALLET/POLY_1271 signature type, got %s/%d", account.WalletType, account.SignatureType)
	}
	result := &DepositWalletOnboardingResult{Account: *account, MinimumPUSD: minimumPUSD}
	if !account.Deployed {
		result.DeployReceipt, err = ops.deploy(ctx)
		if err != nil {
			return result, fmt.Errorf("deploy Deposit Wallet: %w", err)
		}
		account, err = ops.resolve(ctx)
		if err != nil {
			return result, fmt.Errorf("verify Deposit Wallet deployment: %w", err)
		}
		result.Account = *account
		if !account.Deployed {
			return result, fmt.Errorf("Deposit Wallet %s is still not deployed after relayer confirmation", account.Wallet)
		}
	}

	state, pusd, err := ops.state(ctx, common.HexToAddress(string(account.Wallet)))
	if err != nil {
		return result, fmt.Errorf("read onboarding state: %w", err)
	}
	setOnboardingBalances(result, state, pusd)
	if result.PUSDBalance+result.USDCEBalance < minimumPUSD {
		result.Stage = OnboardingStageFundingRequired
		result.Missing = []string{fmt.Sprintf("fund %s with at least %.6f additional pUSD or USDC.e", account.Wallet, minimumPUSD-result.PUSDBalance-result.USDCEBalance)}
		return result, nil
	}

	result.Stage = OnboardingStageApprovalsPending
	result.ApprovalReceipt, err = ops.ensure(ctx)
	if err != nil {
		return result, fmt.Errorf("ensure V2 approvals: %w", err)
	}
	state, pusd, err = ops.state(ctx, common.HexToAddress(string(account.Wallet)))
	if err != nil {
		return result, fmt.Errorf("verify onboarding state: %w", err)
	}
	setOnboardingBalances(result, state, pusd)
	result.Missing = state.missing()
	if result.PUSDBalance < minimumPUSD {
		result.Missing = append(result.Missing, fmt.Sprintf("pUSD balance %.6f is below required %.6f", result.PUSDBalance, minimumPUSD))
	}
	if len(result.Missing) == 0 {
		result.Stage = OnboardingStageReady
		result.Ready = true
	}
	return result, nil
}

func setOnboardingBalances(result *DepositWalletOnboardingResult, state *v2State, pusd *big.Int) {
	result.PUSDBalance = baseUnitsToFloat(pusd)
	result.USDCEBalance = baseUnitsToFloat(state.usdcBal)
}

func baseUnitsToFloat(value *big.Int) float64 {
	if value == nil {
		return 0
	}
	number := new(big.Float).Quo(new(big.Float).SetInt(value), big.NewFloat(1e6))
	result, _ := number.Float64()
	return result
}
