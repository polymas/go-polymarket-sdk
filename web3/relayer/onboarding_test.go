package relayer

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

const (
	onboardingSigner = types.EthAddress("0x1111111111111111111111111111111111111111")
	onboardingWallet = types.EthAddress("0x2222222222222222222222222222222222222222")
)

func TestRunDepositWalletOnboardingStopsForExternalFunding(t *testing.T) {
	ops := &mockOnboardingOps{
		accounts: []*web3.WalletAccount{depositAccount(false), depositAccount(true)},
		states:   []onboardingState{{state: readyV2State(2_000_000), pusd: big.NewInt(0)}},
	}
	result, err := runDepositWalletOnboarding(context.Background(), 5, ops)
	if err != nil {
		t.Fatal(err)
	}
	if result.Stage != OnboardingStageFundingRequired || result.Ready || result.DeployReceipt == nil {
		t.Fatalf("unexpected funding checkpoint: %+v", result)
	}
	if ops.deployCalls != 1 || ops.ensureCalls != 0 {
		t.Fatalf("deploy=%d ensure=%d, want 1/0", ops.deployCalls, ops.ensureCalls)
	}
	if len(result.Missing) != 1 || !strings.Contains(result.Missing[0], string(onboardingWallet)) || !strings.Contains(result.Missing[0], "3.000000") {
		t.Fatalf("unexpected funding instruction: %v", result.Missing)
	}
}

func TestRunDepositWalletOnboardingResumesAndBecomesReady(t *testing.T) {
	ops := &mockOnboardingOps{
		accounts: []*web3.WalletAccount{depositAccount(true)},
		states: []onboardingState{
			{state: missingV2Approvals(10_000_000), pusd: big.NewInt(0)},
			{state: readyV2State(0), pusd: big.NewInt(10_000_000)},
		},
	}
	result, err := runDepositWalletOnboarding(context.Background(), 5, ops)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Ready || result.Stage != OnboardingStageReady || len(result.Missing) != 0 {
		t.Fatalf("unexpected ready result: %+v", result)
	}
	if ops.deployCalls != 0 || ops.ensureCalls != 1 {
		t.Fatalf("deploy=%d ensure=%d, want 0/1", ops.deployCalls, ops.ensureCalls)
	}
	if result.PUSDBalance != 10 || result.USDCEBalance != 0 {
		t.Fatalf("unexpected balances: pUSD=%f USDC.e=%f", result.PUSDBalance, result.USDCEBalance)
	}
}

func TestRunDepositWalletOnboardingReportsPostApprovalMismatch(t *testing.T) {
	stillMissing := missingV2Approvals(0)
	ops := &mockOnboardingOps{
		accounts: []*web3.WalletAccount{depositAccount(true)},
		states: []onboardingState{
			{state: stillMissing, pusd: big.NewInt(5_000_000)},
			{state: stillMissing, pusd: big.NewInt(5_000_000)},
		},
	}
	result, err := runDepositWalletOnboarding(context.Background(), 5, ops)
	if err != nil {
		t.Fatal(err)
	}
	if result.Ready || result.Stage != OnboardingStageApprovalsPending || len(result.Missing) == 0 {
		t.Fatalf("unexpected pending result: %+v", result)
	}
}

func TestRunDepositWalletOnboardingRejectsInvalidInputAndWalletType(t *testing.T) {
	if _, err := runDepositWalletOnboarding(context.Background(), -1, &mockOnboardingOps{}); err == nil {
		t.Fatal("negative minimum should fail")
	}
	ops := &mockOnboardingOps{accounts: []*web3.WalletAccount{{WalletType: web3.WalletTypeGnosisSafe, SignatureType: types.SafeSignatureType}}}
	if _, err := runDepositWalletOnboarding(context.Background(), 0, ops); err == nil {
		t.Fatal("non-deposit wallet should fail")
	}
}

func TestRunDepositWalletOnboardingPropagatesDeployFailure(t *testing.T) {
	ops := &mockOnboardingOps{accounts: []*web3.WalletAccount{depositAccount(false)}, deployErr: errors.New("relayer unavailable")}
	result, err := runDepositWalletOnboarding(context.Background(), 0, ops)
	if err == nil || result == nil || !strings.Contains(err.Error(), "deploy Deposit Wallet") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

type onboardingState struct {
	state *v2State
	pusd  *big.Int
}

type mockOnboardingOps struct {
	accounts    []*web3.WalletAccount
	states      []onboardingState
	deployErr   error
	resolveCall int
	stateCall   int
	deployCalls int
	ensureCalls int
}

func (ops *mockOnboardingOps) resolve(context.Context) (*web3.WalletAccount, error) {
	index := ops.resolveCall
	ops.resolveCall++
	if index >= len(ops.accounts) {
		index = len(ops.accounts) - 1
	}
	return ops.accounts[index], nil
}

func (ops *mockOnboardingOps) deploy(context.Context) (*types.TransactionReceipt, error) {
	ops.deployCalls++
	if ops.deployErr != nil {
		return nil, ops.deployErr
	}
	return &types.TransactionReceipt{Status: 1}, nil
}

func (ops *mockOnboardingOps) state(context.Context, common.Address) (*v2State, *big.Int, error) {
	index := ops.stateCall
	ops.stateCall++
	if index >= len(ops.states) {
		index = len(ops.states) - 1
	}
	return ops.states[index].state, ops.states[index].pusd, nil
}

func (ops *mockOnboardingOps) ensure(context.Context) (*types.TransactionReceipt, error) {
	ops.ensureCalls++
	return &types.TransactionReceipt{Status: 1}, nil
}

func depositAccount(deployed bool) *web3.WalletAccount {
	return &web3.WalletAccount{
		Signer:        onboardingSigner,
		Wallet:        onboardingWallet,
		Funder:        onboardingWallet,
		Maker:         onboardingWallet,
		OrderSigner:   onboardingWallet,
		SignatureType: types.CWIASignatureType,
		WalletType:    web3.WalletTypeDepositWallet,
		Deployed:      deployed,
	}
}

func readyV2State(usdce int64) *v2State {
	state := &v2State{
		usdcBal:     big.NewInt(usdce),
		onrampAllow: big.NewInt(usdce),
		pusdAllow:   make(map[common.Address]*big.Int),
		ctfAppr:     make(map[common.Address]bool),
	}
	for _, spender := range pusdSpendersV2() {
		state.pusdAllow[spender] = big.NewInt(1)
	}
	for _, operator := range ctfOperatorsV2() {
		state.ctfAppr[operator] = true
	}
	return state
}

func missingV2Approvals(usdce int64) *v2State {
	return &v2State{
		usdcBal:     big.NewInt(usdce),
		onrampAllow: big.NewInt(0),
		pusdAllow:   make(map[common.Address]*big.Int),
		ctfAppr:     make(map[common.Address]bool),
	}
}
