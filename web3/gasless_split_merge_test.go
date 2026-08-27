package web3

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	sdkerrors "github.com/polymas/go-polymarket-sdk/errors"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

const splitMergeTestConditionID = types.Keccak256("0x1111111111111111111111111111111111111111111111111111111111111111")

func TestToUnits6(t *testing.T) {
	tests := []struct {
		name   string
		amount float64
		want   string
	}{
		{name: "one micro", amount: 0.000001, want: "1"},
		{name: "one", amount: 1, want: "1000000"},
		{name: "six decimal boundary", amount: 1.000001, want: "1000001"},
		{name: "ordinary amount", amount: 5.123456, want: "5123456"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toUnits6(tt.amount)
			if err != nil {
				t.Fatalf("toUnits6(%v): %v", tt.amount, err)
			}
			if got.String() != tt.want {
				t.Fatalf("toUnits6(%v) = %s, want %s", tt.amount, got, tt.want)
			}
		})
	}
}

func TestToUnits6RejectsInvalidAmounts(t *testing.T) {
	tests := []struct {
		name    string
		amount  float64
		wantErr string
	}{
		{name: "zero", amount: 0, wantErr: "positive"},
		{name: "negative", amount: -1, wantErr: "positive"},
		{name: "NaN", amount: math.NaN(), wantErr: "positive"},
		{name: "positive infinity", amount: math.Inf(1), wantErr: "positive"},
		{name: "negative infinity", amount: math.Inf(-1), wantErr: "positive"},
		{name: "less than one micro", amount: 0.0000001, wantErr: "at most 6 decimal places"},
		{name: "more than six decimals", amount: 1.0000019, wantErr: "at most 6 decimal places"},
		{name: "uint256 overflow", amount: math.MaxFloat64, wantErr: "exceeds uint256"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toUnits6(tt.amount)
			if err == nil {
				t.Fatalf("toUnits6(%v) = %v, want error containing %q", tt.amount, got, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("toUnits6(%v) error = %q, want substring %q", tt.amount, err, tt.wantErr)
			}
		})
	}
}

func TestSplitMergeCalldataMatchesSolidityABI(t *testing.T) {
	const ctfABI = `[
{"name":"splitPosition","type":"function","stateMutability":"nonpayable","inputs":[{"name":"collateralToken","type":"address"},{"name":"parentCollectionId","type":"bytes32"},{"name":"conditionId","type":"bytes32"},{"name":"partition","type":"uint256[]"},{"name":"amount","type":"uint256"}],"outputs":[]},
{"name":"mergePositions","type":"function","stateMutability":"nonpayable","inputs":[{"name":"collateralToken","type":"address"},{"name":"parentCollectionId","type":"bytes32"},{"name":"conditionId","type":"bytes32"},{"name":"partition","type":"uint256[]"},{"name":"amount","type":"uint256"}],"outputs":[]}
]`
	parsed, err := abi.JSON(strings.NewReader(ctfABI))
	if err != nil {
		t.Fatalf("parse test ABI: %v", err)
	}

	amount := big.NewInt(1_234_567)
	args := []any{
		common.HexToAddress(internal.PolygonCollateral),
		common.HexToHash(internal.HashZero),
		common.HexToHash(string(splitMergeTestConditionID)),
		[]*big.Int{big.NewInt(1), big.NewInt(2)},
		amount,
	}
	tests := []struct {
		method string
		encode func() ([]byte, error)
	}{
		{method: "splitPosition", encode: func() ([]byte, error) {
			return (&GaslessClient{}).encodeSplit(splitMergeTestConditionID, amount)
		}},
		{method: "mergePositions", encode: func() ([]byte, error) {
			return (&GaslessClient{}).encodeMerge(splitMergeTestConditionID, amount)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			want, err := parsed.Pack(tt.method, args...)
			if err != nil {
				t.Fatalf("ABI pack %s: %v", tt.method, err)
			}
			got, err := tt.encode()
			if err != nil {
				t.Fatalf("encode %s: %v", tt.method, err)
			}
			if hex.EncodeToString(got) != hex.EncodeToString(want) {
				t.Fatalf("%s calldata mismatch:\nwant %x\n got %x", tt.method, want, got)
			}
		})
	}
}

func TestAdapterAddrV2(t *testing.T) {
	tests := []struct {
		name    string
		negRisk bool
		want    string
	}{
		{name: "regular", want: internal.PolygonCtfCollateralAdapter},
		{name: "neg risk", negRisk: true, want: internal.PolygonNegRiskCtfCollateralAdapter},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adapterAddrV2(tt.negRisk)
			if got != common.HexToAddress(tt.want) {
				t.Fatalf("adapterAddrV2(%v) = %s, want %s", tt.negRisk, got.Hex(), tt.want)
			}
		})
	}
}

func TestSplitMergeValidateBeforeRelay(t *testing.T) {
	c := &GaslessClient{}
	invalidCondition := types.Keccak256("invalid")

	if _, err := c.SplitPosition(1, invalidCondition, false); err == nil || !strings.Contains(err.Error(), "conditionID") {
		t.Fatalf("SplitPosition invalid condition error = %v", err)
	}
	if _, err := c.MergePositions(invalidCondition, 1, false); err == nil || !strings.Contains(err.Error(), "conditionID") {
		t.Fatalf("MergePositions invalid condition error = %v", err)
	}
	if _, err := c.SplitPosition(0, splitMergeTestConditionID, false); err == nil || !strings.Contains(err.Error(), "positive") {
		t.Fatalf("SplitPosition zero amount error = %v", err)
	}
	if _, err := c.MergePositions(splitMergeTestConditionID, math.NaN(), false); err == nil || !strings.Contains(err.Error(), "positive") {
		t.Fatalf("MergePositions NaN amount error = %v", err)
	}
}

type splitMergeProxyCall struct {
	TypeCode uint8
	To       common.Address
	Value    *big.Int
	Data     []byte
}

func decodeSingleProxyCall(t *testing.T, encoded string) splitMergeProxyCall {
	t.Helper()
	const proxyABI = `[{"name":"proxy","type":"function","stateMutability":"nonpayable","inputs":[{"name":"calls","type":"tuple[]","components":[{"name":"typeCode","type":"uint8"},{"name":"to","type":"address"},{"name":"value","type":"uint256"},{"name":"data","type":"bytes"}]}],"outputs":[]}]`
	parsed, err := abi.JSON(strings.NewReader(proxyABI))
	if err != nil {
		t.Fatalf("parse proxy ABI: %v", err)
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(encoded, "0x"))
	if err != nil {
		t.Fatalf("decode proxy calldata: %v", err)
	}
	if len(raw) < 4 || !strings.EqualFold(hex.EncodeToString(raw[:4]), hex.EncodeToString(parsed.Methods["proxy"].ID)) {
		t.Fatalf("unexpected proxy selector: %x", raw)
	}
	values, err := parsed.Methods["proxy"].Inputs.Unpack(raw[4:])
	if err != nil {
		t.Fatalf("unpack proxy calldata: %v", err)
	}
	calls := *abi.ConvertType(values[0], new([]splitMergeProxyCall)).(*[]splitMergeProxyCall)
	if len(calls) != 1 {
		t.Fatalf("proxy call count = %d, want 1", len(calls))
	}
	return calls[0]
}

func TestSplitMergeRelayPayload(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		metadata   string
		negRisk    bool
		invoke     func(*GaslessClient) error
		encodeWant func(*GaslessClient, *big.Int) ([]byte, error)
	}{
		{
			name: "split regular", method: "split", metadata: "split-v2",
			invoke: func(c *GaslessClient) error {
				_, err := c.SplitPosition(1.25, splitMergeTestConditionID, false)
				return err
			},
			encodeWant: func(c *GaslessClient, amount *big.Int) ([]byte, error) {
				return c.encodeSplit(splitMergeTestConditionID, amount)
			},
		},
		{
			name: "split neg risk", method: "split", metadata: "split-v2", negRisk: true,
			invoke: func(c *GaslessClient) error {
				_, err := c.SplitPosition(1.25, splitMergeTestConditionID, true)
				return err
			},
			encodeWant: func(c *GaslessClient, amount *big.Int) ([]byte, error) {
				return c.encodeSplit(splitMergeTestConditionID, amount)
			},
		},
		{
			name: "merge regular", method: "merge", metadata: "merge-v2",
			invoke: func(c *GaslessClient) error {
				_, err := c.MergePositions(splitMergeTestConditionID, 1.25, false)
				return err
			},
			encodeWant: func(c *GaslessClient, amount *big.Int) ([]byte, error) {
				return c.encodeMerge(splitMergeTestConditionID, amount)
			},
		},
		{
			name: "merge neg risk", method: "merge", metadata: "merge-v2", negRisk: true,
			invoke: func(c *GaslessClient) error {
				_, err := c.MergePositions(splitMergeTestConditionID, 1.25, true)
				return err
			},
			encodeWant: func(c *GaslessClient, amount *big.Int) ([]byte, error) {
				return c.encodeMerge(splitMergeTestConditionID, amount)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var submits int64
			var body ProxyRelayBody
			relayer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasPrefix(r.URL.Path, "/nonce"):
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"nonce":"0"}`)
				case r.URL.Path == "/submit":
					atomic.AddInt64(&submits, 1)
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Errorf("decode submit body: %v", err)
					}
					w.WriteHeader(http.StatusUnprocessableEntity)
					fmt.Fprint(w, `{"error":"unit-test stop before chain submission"}`)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(relayer.Close)

			client := newRetryTestClient(t, relayer.URL)
			err := tt.invoke(client)
			var relayErr *sdkerrors.RelayHTTPError
			if err == nil || !strings.Contains(err.Error(), "unit-test stop") || !asRelayHTTPError(err, &relayErr) {
				t.Fatalf("%s error = %v, want mocked RelayHTTPError", tt.method, err)
			}
			if got := atomic.LoadInt64(&submits); got != 1 {
				t.Fatalf("submit calls = %d, want 1", got)
			}
			if body.Metadata != tt.metadata || body.Type != "PROXY" {
				t.Fatalf("relay body metadata/type = %q/%q, want %q/PROXY", body.Metadata, body.Type, tt.metadata)
			}

			call := decodeSingleProxyCall(t, body.Data)
			if call.TypeCode != 1 || call.Value.Sign() != 0 {
				t.Fatalf("proxy call type/value = %d/%s, want 1/0", call.TypeCode, call.Value)
			}
			wantAdapter := adapterAddrV2(tt.negRisk)
			if call.To != wantAdapter {
				t.Fatalf("proxy target = %s, want %s", call.To.Hex(), wantAdapter.Hex())
			}
			wantData, err := tt.encodeWant(client, big.NewInt(1_250_000))
			if err != nil {
				t.Fatalf("encode expected calldata: %v", err)
			}
			if hex.EncodeToString(call.Data) != hex.EncodeToString(wantData) {
				t.Fatalf("inner calldata mismatch:\nwant %x\n got %x", wantData, call.Data)
			}
		})
	}
}

func asRelayHTTPError(err error, target **sdkerrors.RelayHTTPError) bool {
	for err != nil {
		if typed, ok := err.(*sdkerrors.RelayHTTPError); ok {
			*target = typed
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// TestSplitMergeRoundTripIntegration 是显式启用的 Polygon 主网往返测试。
// 默认 go test 永远不会触发真实交易；只有同时提供开关、私钥和 condition ID 才执行。
// Split 成功后立即对同一 condition/amount Merge，净资产理论上只发生形态往返。
func TestSplitMergeRoundTripIntegration(t *testing.T) {
	if os.Getenv("POLY_RUN_SPLIT_MERGE_INTEGRATION") != "1" {
		t.Skip("set POLY_RUN_SPLIT_MERGE_INTEGRATION=1 to enable real split/merge transactions")
	}
	privateKey := os.Getenv("POLY_PRIVATE_KEY")
	if privateKey == "" {
		privateKey = os.Getenv("poly_sec")
	}
	if privateKey == "" {
		t.Fatal("POLY_PRIVATE_KEY or poly_sec is required")
	}
	conditionID := types.Keccak256(os.Getenv("POLY_TEST_CONDITION_ID"))
	if err := conditionID.Validate(); err != nil {
		t.Fatalf("valid POLY_TEST_CONDITION_ID is required: %v", err)
	}

	amount := 0.01
	if raw := os.Getenv("POLY_TEST_SPLIT_MERGE_AMOUNT"); raw != "" {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			t.Fatalf("parse POLY_TEST_SPLIT_MERGE_AMOUNT: %v", err)
		}
		amount = parsed
	}
	negRisk, err := strconv.ParseBool(defaultString(os.Getenv("POLY_TEST_NEG_RISK"), "false"))
	if err != nil {
		t.Fatalf("parse POLY_TEST_NEG_RISK: %v", err)
	}
	signatureTypeInt, err := strconv.Atoi(defaultString(os.Getenv("POLY_SIGNATURE_TYPE"), strconv.Itoa(int(types.SafeSignatureType))))
	if err != nil {
		t.Fatalf("parse POLY_SIGNATURE_TYPE: %v", err)
	}

	client, err := NewGaslessClient(privateKey, types.SignatureType(signatureTypeInt), types.Polygon, nil)
	if err != nil {
		t.Fatalf("NewGaslessClient: %v", err)
	}
	requiredUnits, err := toUnits6(amount)
	if err != nil {
		t.Fatalf("invalid integration amount: %v", err)
	}
	walletString, err := client.GetPolyProxyAddress()
	if err != nil {
		t.Fatalf("GetPolyProxyAddress: %v", err)
	}
	wallet := common.HexToAddress(string(walletString))
	adapter := adapterAddrV2(negRisk)
	pusd := common.HexToAddress(internal.PolygonPUSD)
	pusdBalance, err := client.callERC20Uint(context.Background(), pusd, "balanceOf", wallet)
	if err != nil {
		t.Fatalf("read pUSD balance: %v", err)
	}
	pusdAllowance, err := client.callERC20Uint(context.Background(), pusd, "allowance", wallet, adapter)
	if err != nil {
		t.Fatalf("read pUSD adapter allowance: %v", err)
	}
	ctfApproved, err := readCTFApprovalForTest(client, wallet, adapter)
	if err != nil {
		t.Fatalf("read CTF adapter approval: %v", err)
	}
	t.Logf("preflight wallet=%s adapter=%s pUSD=%s allowance=%s ctfApproved=%v",
		wallet.Hex(), adapter.Hex(), pusdBalance, pusdAllowance, ctfApproved)
	if pusdBalance.Cmp(requiredUnits) < 0 {
		t.Fatalf("insufficient pUSD: balance=%s required=%s", pusdBalance, requiredUnits)
	}
	if pusdAllowance.Cmp(requiredUnits) < 0 {
		if os.Getenv("POLY_TEST_AUTO_APPROVE_EXACT") != "1" {
			t.Fatalf("insufficient pUSD allowance to adapter %s: allowance=%s required=%s; set POLY_TEST_AUTO_APPROVE_EXACT=1 to approve exactly this test amount", adapter.Hex(), pusdAllowance, requiredUnits)
		}
		approveData, err := erc20ABI.Pack("approve", adapter, requiredUnits)
		if err != nil {
			t.Fatalf("pack exact adapter approval: %v", err)
		}
		approveReceipt, err := client.executeGaslessBatch(
			[]map[string]any{callTxn(pusd, approveData)},
			"Approve exact split/merge test amount",
			"approve-split-merge-test",
		)
		if err != nil {
			t.Fatalf("approve exact pUSD allowance for adapter %s: %v", adapter.Hex(), err)
		}
		assertSuccessfulReceipt(t, "approve exact", approveReceipt)
		t.Logf("exact approval confirmed: tx=%s block=%d amount=%s", approveReceipt.TxHash, approveReceipt.BlockNumber, requiredUnits)

		// 如果后续 Split 没有消费 allowance，测试退出时尽力清零，避免留下长期授权。
		defer func() {
			remaining, readErr := client.callERC20Uint(context.Background(), pusd, "allowance", wallet, adapter)
			if readErr != nil || remaining.Sign() == 0 {
				return
			}
			clearData, packErr := erc20ABI.Pack("approve", adapter, big.NewInt(0))
			if packErr != nil {
				t.Errorf("pack cleanup approval: %v", packErr)
				return
			}
			clearReceipt, clearErr := client.executeGaslessBatch(
				[]map[string]any{callTxn(pusd, clearData)},
				"Clear split/merge test allowance",
				"clear-split-merge-test-allowance",
			)
			if clearErr != nil {
				t.Errorf("cleanup remaining adapter allowance %s: %v", remaining, clearErr)
				return
			}
			assertSuccessfulReceipt(t, "clear exact approval", clearReceipt)
		}()
	}
	if !ctfApproved {
		t.Fatalf("CTF adapter %s is not approved; merge would revert after split", adapter.Hex())
	}

	splitReceipt, err := client.SplitPosition(amount, conditionID, negRisk)
	if err != nil {
		t.Fatalf("SplitPosition(%f): %v", amount, err)
	}
	assertSuccessfulReceipt(t, "split", splitReceipt)
	t.Logf("split confirmed: tx=%s block=%d", splitReceipt.TxHash, splitReceipt.BlockNumber)

	mergeReceipt, err := client.MergePositions(conditionID, amount, negRisk)
	if err != nil {
		t.Fatalf("MergePositions(%f) after successful split (position tokens remain recoverable): %v", amount, err)
	}
	assertSuccessfulReceipt(t, "merge", mergeReceipt)
	t.Logf("merge confirmed: tx=%s block=%d", mergeReceipt.TxHash, mergeReceipt.BlockNumber)

	finalBalance, err := client.callERC20Uint(context.Background(), pusd, "balanceOf", wallet)
	if err != nil {
		t.Fatalf("read final pUSD balance: %v", err)
	}
	if finalBalance.Cmp(pusdBalance) != 0 {
		t.Fatalf("pUSD balance did not round-trip exactly: before=%s after=%s", pusdBalance, finalBalance)
	}
	t.Logf("round trip balance verified: before=%s after=%s", pusdBalance, finalBalance)
}

func readCTFApprovalForTest(client *GaslessClient, wallet, adapter common.Address) (bool, error) {
	ctf := common.HexToAddress(internal.PolygonConditionalTokens)
	calldata, err := isApprovedForAllAB.Pack("isApprovedForAll", wallet, adapter)
	if err != nil {
		return false, err
	}
	out, err := client.callContractWithRetry(context.Background(), ethereum.CallMsg{To: &ctf, Data: calldata}, nil)
	if err != nil {
		return false, err
	}
	var approved bool
	if err := isApprovedForAllAB.UnpackIntoInterface(&approved, "isApprovedForAll", out); err != nil {
		return false, err
	}
	return approved, nil
}

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func assertSuccessfulReceipt(t *testing.T, operation string, receipt *types.TransactionReceipt) {
	t.Helper()
	if receipt == nil {
		t.Fatalf("%s returned nil receipt", operation)
	}
	if receipt.Status != 1 {
		t.Fatalf("%s receipt status = %d, want 1 (tx=%s)", operation, receipt.Status, receipt.TxHash)
	}
}
