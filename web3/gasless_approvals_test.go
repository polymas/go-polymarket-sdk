package web3

import (
	"context"
	"math/big"
	"os"
	"strconv"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

func TestBuildV2AllowanceTransactionsUsesPUSD(t *testing.T) {
	txns, err := buildV2AllowanceTransactions()
	if err != nil {
		t.Fatalf("buildV2AllowanceTransactions: %v", err)
	}
	if len(txns) != 4 {
		t.Fatalf("transactions = %d, want 4", len(txns))
	}

	pusd := common.HexToAddress(internal.PolygonPUSD)
	usdc := common.HexToAddress(internal.PolygonCollateral)
	ctf := common.HexToAddress(internal.PolygonConditionalTokens)
	spenders := []common.Address{
		common.HexToAddress(internal.PolygonExchangeV2),
		common.HexToAddress(internal.PolygonNegRiskExchangeV2),
	}

	for i, spender := range spenders {
		target, data := approvalTestCall(t, txns[i])
		if target == usdc {
			t.Fatalf("transaction %d targets legacy USDC.e collateral", i)
		}
		if target != pusd {
			t.Fatalf("transaction %d target = %s, want pUSD %s", i, target.Hex(), pusd.Hex())
		}
		if len(data) < 4 || string(data[:4]) != string(erc20ABI.Methods["approve"].ID) {
			t.Fatalf("transaction %d is not ERC20.approve", i)
		}
		args, err := erc20ABI.Methods["approve"].Inputs.Unpack(data[4:])
		if err != nil {
			t.Fatalf("unpack approve %d: %v", i, err)
		}
		if got := args[0].(common.Address); got != spender {
			t.Fatalf("approve %d spender = %s, want %s", i, got.Hex(), spender.Hex())
		}
		if got := args[1].(*big.Int); got.Cmp(maxUint256) != 0 {
			t.Fatalf("approve %d amount = %s, want max uint256", i, got)
		}
	}

	for i, operator := range spenders {
		target, data := approvalTestCall(t, txns[i+2])
		if target != ctf {
			t.Fatalf("transaction %d target = %s, want CTF %s", i+2, target.Hex(), ctf.Hex())
		}
		method := setApprovalForAllAB.Methods["setApprovalForAll"]
		if len(data) < 4 || string(data[:4]) != string(method.ID) {
			t.Fatalf("transaction %d is not CTF.setApprovalForAll", i+2)
		}
		args, err := method.Inputs.Unpack(data[4:])
		if err != nil {
			t.Fatalf("unpack setApprovalForAll %d: %v", i, err)
		}
		if got := args[0].(common.Address); got != operator {
			t.Fatalf("setApprovalForAll %d operator = %s, want %s", i, got.Hex(), operator.Hex())
		}
		if approved := args[1].(bool); !approved {
			t.Fatalf("setApprovalForAll %d approved = false", i)
		}
	}
}

func TestSetV2AllowancesIntegration(t *testing.T) {
	if os.Getenv("POLY_RUN_V2_ALLOWANCE_INTEGRATION") != "1" {
		t.Skip("set POLY_RUN_V2_ALLOWANCE_INTEGRATION=1 to enable real allowance transaction")
	}
	privateKey := os.Getenv("POLY_PRIVATE_KEY")
	if privateKey == "" {
		privateKey = os.Getenv("poly_sec")
	}
	if privateKey == "" {
		t.Fatal("POLY_PRIVATE_KEY or poly_sec is required")
	}
	signatureType, err := strconv.Atoi(defaultString(os.Getenv("POLY_SIGNATURE_TYPE"), strconv.Itoa(int(types.SafeSignatureType))))
	if err != nil {
		t.Fatalf("parse POLY_SIGNATURE_TYPE: %v", err)
	}

	var rpcURLs []string
	if rpcURL := os.Getenv("POLY_TEST_RPC_URL"); rpcURL != "" {
		rpcURLs = []string{rpcURL}
	}
	client, err := NewGaslessClient(privateKey, types.SignatureType(signatureType), types.Polygon, nil, rpcURLs...)
	if err != nil {
		t.Fatalf("NewGaslessClient: %v", err)
	}
	walletString, err := client.GetPolyProxyAddress()
	if err != nil {
		t.Fatalf("GetPolyProxyAddress: %v", err)
	}
	wallet := common.HexToAddress(string(walletString))
	usdc := common.HexToAddress(internal.PolygonCollateral)
	pusd := common.HexToAddress(internal.PolygonPUSD)
	spenders := pusdSpendersV2()

	usdcBefore := make([]*big.Int, len(spenders))
	for i, spender := range spenders {
		usdcBefore[i], err = client.callERC20Uint(context.Background(), usdc, "allowance", wallet, spender)
		if err != nil {
			t.Fatalf("read USDC.e allowance before transaction: %v", err)
		}
	}

	receipt, err := client.SetV2Allowances()
	if err != nil {
		t.Fatalf("SetV2Allowances: %v", err)
	}
	assertSuccessfulReceipt(t, "SetV2Allowances", receipt)

	for i, spender := range spenders {
		pusdAfter, err := client.callERC20Uint(context.Background(), pusd, "allowance", wallet, spender)
		if err != nil {
			t.Fatalf("read pUSD allowance after transaction: %v", err)
		}
		if pusdAfter.Cmp(maxUint256) != 0 {
			t.Fatalf("pUSD allowance to %s = %s, want max uint256", spender.Hex(), pusdAfter)
		}

		usdcAfter, err := client.callERC20Uint(context.Background(), usdc, "allowance", wallet, spender)
		if err != nil {
			t.Fatalf("read USDC.e allowance after transaction: %v", err)
		}
		if usdcAfter.Cmp(usdcBefore[i]) != 0 {
			t.Fatalf("USDC.e allowance to %s changed: before=%s after=%s", spender.Hex(), usdcBefore[i], usdcAfter)
		}

		approved, err := readCTFApprovalForTest(client, wallet, spender)
		if err != nil {
			t.Fatalf("read CTF approval for %s: %v", spender.Hex(), err)
		}
		if !approved {
			t.Fatalf("CTF approval for %s = false", spender.Hex())
		}
	}
	t.Logf("V2 allowances confirmed: tx=%s block=%d", receipt.TxHash, receipt.BlockNumber)
}

func approvalTestCall(t *testing.T, txn map[string]any) (common.Address, []byte) {
	t.Helper()
	targetRaw, ok := txn["to"].(string)
	if !ok {
		t.Fatalf("transaction target has type %T", txn["to"])
	}
	dataRaw, ok := txn["data"].(string)
	if !ok {
		t.Fatalf("transaction data has type %T", txn["data"])
	}
	return common.HexToAddress(targetRaw), common.FromHex(dataRaw)
}
