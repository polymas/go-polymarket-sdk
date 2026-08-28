package web3

import (
	"context"
	"fmt"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// WalletType is the account wallet model. Values match Polymarket's current
// AccountIdentity WalletType enum and the corresponding SignatureType.
type WalletType uint8

const (
	WalletTypeEOA           WalletType = 0
	WalletTypePolyProxy     WalletType = 1
	WalletTypeGnosisSafe    WalletType = 2
	WalletTypeDepositWallet WalletType = 3
)

func (walletType WalletType) String() string {
	switch walletType {
	case WalletTypeEOA:
		return "EOA"
	case WalletTypePolyProxy:
		return "POLY_PROXY"
	case WalletTypeGnosisSafe:
		return "GNOSIS_SAFE"
	case WalletTypeDepositWallet:
		return "DEPOSIT_WALLET"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", walletType)
	}
}

// WalletAccount makes the SDK's signer/funder semantics explicit.
// Signer is the controlling EOA; Wallet/Funder/Maker hold trading assets.
// OrderSigner is the signer field written into a V2 order. For a Deposit
// Wallet it equals Wallet because the order is verified through EIP-1271.
type WalletAccount struct {
	Signer        types.EthAddress
	Wallet        types.EthAddress
	Funder        types.EthAddress
	Maker         types.EthAddress
	OrderSigner   types.EthAddress
	SignatureType types.SignatureType
	WalletType    WalletType
	Deployed      bool
}

// ResolveWalletAccount resolves the effective account identity and checks
// whether a smart-account wallet currently has bytecode on Polygon.
func (c *baseClient) ResolveWalletAccount(ctx context.Context) (*WalletAccount, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	wallet, err := c.GetPolyProxyAddress()
	if err != nil {
		return nil, fmt.Errorf("resolve account wallet: %w", err)
	}
	account := &WalletAccount{
		Signer:        c.baseAddress,
		Wallet:        wallet,
		Funder:        wallet,
		Maker:         wallet,
		OrderSigner:   c.baseAddress,
		SignatureType: c.signatureType,
		WalletType:    WalletType(c.signatureType),
		Deployed:      c.signatureType == types.EOASignatureType,
	}
	if c.signatureType == types.CWIASignatureType {
		account.OrderSigner = wallet
	}
	if c.signatureType != types.EOASignatureType {
		deployed, err := c.IsDepositWalletDeployed(ctx, common.HexToAddress(string(wallet)))
		if err != nil {
			return nil, fmt.Errorf("check %s deployment: %w", account.WalletType, err)
		}
		account.Deployed = deployed
	}
	return account, nil
}

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
	Account         WalletAccount
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
	resolve(context.Context) (*WalletAccount, error)
	deploy() (*types.TransactionReceipt, error)
	state(context.Context, common.Address) (*v2State, *big.Int, error)
	ensure() (*types.TransactionReceipt, error)
}

type gaslessDepositWalletOnboardingOps struct{ client *GaslessClient }

func (ops gaslessDepositWalletOnboardingOps) resolve(ctx context.Context) (*WalletAccount, error) {
	return ops.client.ResolveWalletAccount(ctx)
}

func (ops gaslessDepositWalletOnboardingOps) deploy() (*types.TransactionReceipt, error) {
	return ops.client.DeployDepositWallet(false)
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

func (ops gaslessDepositWalletOnboardingOps) ensure() (*types.TransactionReceipt, error) {
	return ops.client.EnsureV2Ready()
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
	if account.WalletType != WalletTypeDepositWallet || account.SignatureType != types.CWIASignatureType {
		return nil, fmt.Errorf("OnboardDepositWallet requires DEPOSIT_WALLET/CWIASignatureType, got %s/%d", account.WalletType, account.SignatureType)
	}
	result := &DepositWalletOnboardingResult{Account: *account, MinimumPUSD: minimumPUSD}
	if !account.Deployed {
		result.DeployReceipt, err = ops.deploy()
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
	result.ApprovalReceipt, err = ops.ensure()
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
