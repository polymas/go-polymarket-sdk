package web3

import (
	"context"
	"errors"
	"math/big"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/polymas/go-polymarket-sdk/types"
)

const (
	onboardingSigner = types.EthAddress("0x1111111111111111111111111111111111111111")
	onboardingWallet = types.EthAddress("0x2222222222222222222222222222222222222222")
)

func TestResolveWalletAccountSemantics(t *testing.T) {
	tests := []struct {
		name            string
		signatureType   types.SignatureType
		wallet          types.EthAddress
		wantType        WalletType
		wantOrderSigner types.EthAddress
	}{
		{"EOA", types.EOASignatureType, onboardingSigner, WalletTypeEOA, onboardingSigner},
		{"Proxy", types.ProxySignatureType, onboardingWallet, WalletTypePolyProxy, onboardingSigner},
		{"Safe", types.SafeSignatureType, onboardingWallet, WalletTypeGnosisSafe, onboardingSigner},
		{"DepositWallet", types.CWIASignatureType, onboardingWallet, WalletTypeDepositWallet, onboardingWallet},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &baseClient{
				baseAddress:   onboardingSigner,
				proxyAddress:  test.wallet,
				signatureType: test.signatureType,
				depositWalletRPC: &depositWalletRPC{code: func(context.Context, common.Address) ([]byte, error) {
					return []byte{0x60}, nil
				}},
			}
			account, err := client.ResolveWalletAccount(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if account.Signer != onboardingSigner || account.Wallet != test.wallet || account.Funder != test.wallet || account.Maker != test.wallet {
				t.Fatalf("unexpected account addresses: %+v", account)
			}
			if account.WalletType != test.wantType || account.OrderSigner != test.wantOrderSigner || !account.Deployed {
				t.Fatalf("unexpected account semantics: %+v", account)
			}
		})
	}
}

func TestRunDepositWalletOnboardingStopsForExternalFunding(t *testing.T) {
	ops := &mockOnboardingOps{
		accounts: []*WalletAccount{depositAccount(false), depositAccount(true)},
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
		accounts: []*WalletAccount{depositAccount(true)},
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
		accounts: []*WalletAccount{depositAccount(true)},
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
	ops := &mockOnboardingOps{accounts: []*WalletAccount{{WalletType: WalletTypeGnosisSafe, SignatureType: types.SafeSignatureType}}}
	if _, err := runDepositWalletOnboarding(context.Background(), 0, ops); err == nil {
		t.Fatal("non-deposit wallet should fail")
	}
}

func TestRunDepositWalletOnboardingPropagatesDeployFailure(t *testing.T) {
	ops := &mockOnboardingOps{accounts: []*WalletAccount{depositAccount(false)}, deployErr: errors.New("relayer unavailable")}
	result, err := runDepositWalletOnboarding(context.Background(), 0, ops)
	if err == nil || result == nil || !strings.Contains(err.Error(), "deploy Deposit Wallet") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestResolveWalletAccountLive(t *testing.T) {
	if testing.Short() || os.Getenv("POLY_RUN_ONBOARDING_LIVE") != "1" {
		t.Skip("set POLY_RUN_ONBOARDING_LIVE=1 to run read-only Polygon account resolution")
	}
	privateKey := os.Getenv("POLY_PRIVATE_KEY")
	if privateKey == "" {
		t.Skip("POLY_PRIVATE_KEY is not configured")
	}
	signatureType, err := strconv.Atoi(os.Getenv("POLY_SIGNATURE_TYPE"))
	if err != nil {
		t.Fatalf("invalid POLY_SIGNATURE_TYPE: %v", err)
	}
	client, err := NewClient(privateKey, types.SignatureType(signatureType), types.Polygon)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	account, err := client.ResolveWalletAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if account.Signer == "" || account.Wallet == "" || account.Funder != account.Wallet || account.Maker != account.Wallet {
		t.Fatalf("invalid resolved account: %+v", account)
	}
	if account.SignatureType != types.SignatureType(signatureType) || account.WalletType != WalletType(signatureType) {
		t.Fatalf("resolved type mismatch: %+v", account)
	}
	if signatureType != int(types.EOASignatureType) && !account.Deployed {
		t.Fatalf("configured production account wallet is not deployed: %s", account.Wallet)
	}
}

type onboardingState struct {
	state *v2State
	pusd  *big.Int
}

type mockOnboardingOps struct {
	accounts    []*WalletAccount
	states      []onboardingState
	deployErr   error
	resolveCall int
	stateCall   int
	deployCalls int
	ensureCalls int
}

func (ops *mockOnboardingOps) resolve(context.Context) (*WalletAccount, error) {
	index := ops.resolveCall
	ops.resolveCall++
	if index >= len(ops.accounts) {
		index = len(ops.accounts) - 1
	}
	return ops.accounts[index], nil
}

func (ops *mockOnboardingOps) deploy() (*types.TransactionReceipt, error) {
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

func (ops *mockOnboardingOps) ensure() (*types.TransactionReceipt, error) {
	ops.ensureCalls++
	return &types.TransactionReceipt{Status: 1}, nil
}

func depositAccount(deployed bool) *WalletAccount {
	return &WalletAccount{
		Signer:        onboardingSigner,
		Wallet:        onboardingWallet,
		Funder:        onboardingWallet,
		Maker:         onboardingWallet,
		OrderSigner:   onboardingWallet,
		SignatureType: types.CWIASignatureType,
		WalletType:    WalletTypeDepositWallet,
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
