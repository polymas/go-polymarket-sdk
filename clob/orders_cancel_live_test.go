package clob

import (
	"os"
	"strconv"
	"testing"

	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

// TestCancelMarketOrderFiltersLive 对生产 CLOB 验证 condition-only、token-only
// 和 condition+token 三种撤单过滤请求。测试先读取全部开放订单并拒绝命中任何
// 现有订单，因此只验证协议接受情况，不创建订单，也不改变账户状态。
func TestCancelMarketOrderFiltersLive(t *testing.T) {
	if os.Getenv("POLY_RUN_CANCEL_FILTER_INTEGRATION") != "1" {
		t.Skip("set POLY_RUN_CANCEL_FILTER_INTEGRATION=1 to enable the live cancel-filter probe")
	}
	conditionText := os.Getenv("POLY_CANCEL_FILTER_TEST_CONDITION")
	tokenText := os.Getenv("POLY_CANCEL_FILTER_TEST_TOKEN")
	if conditionText == "" || tokenText == "" {
		t.Fatal("POLY_CANCEL_FILTER_TEST_CONDITION and POLY_CANCEL_FILTER_TEST_TOKEN are required")
	}
	privateKey := os.Getenv("POLY_PRIVATE_KEY")
	if privateKey == "" {
		privateKey = os.Getenv("poly_sec")
	}
	if privateKey == "" {
		t.Fatal("POLY_PRIVATE_KEY or poly_sec is required")
	}
	signatureType, err := strconv.Atoi(defaultEnv("POLY_SIGNATURE_TYPE", "2"))
	if err != nil {
		t.Fatalf("parse POLY_SIGNATURE_TYPE: %v", err)
	}
	conditionID, err := types.ParseConditionID(conditionText)
	if err != nil {
		t.Fatalf("parse condition ID: %v", err)
	}
	tokenID, err := types.ParseTokenID(tokenText)
	if err != nil {
		t.Fatalf("parse token ID: %v", err)
	}

	w3, err := web3.NewClient(privateKey, types.SignatureType(signatureType), types.Polygon)
	if err != nil {
		t.Fatalf("create web3 client: %v", err)
	}
	defer w3.Close()
	client, err := NewClient(w3)
	if err != nil {
		t.Fatalf("create CLOB client: %v", err)
	}

	book, err := client.GetOrderBook(tokenID.String())
	if err != nil {
		t.Fatalf("verify active token: %v", err)
	}
	if book.Market != conditionID.String() {
		t.Fatalf("token belongs to market %s, want %s", book.Market, conditionID)
	}
	openOrders, err := client.GetOrders(nil, nil, nil)
	if err != nil {
		t.Fatalf("safety check open orders: %v", err)
	}
	for _, order := range openOrders {
		if order.ConditionID.String() == conditionID.String() || order.TokenID == tokenID.String() {
			t.Fatalf("safety check: filter would cancel existing order %s", order.OrderID)
		}
	}

	tests := []struct {
		name   string
		params types.CancelMarketOrdersParams
	}{
		{name: "condition-only", params: types.CancelMarketOrdersParams{ConditionID: conditionID}},
		{name: "token-only", params: types.CancelMarketOrdersParams{TokenID: tokenID}},
		{name: "condition-and-token", params: types.CancelMarketOrdersParams{ConditionID: conditionID, TokenID: tokenID}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, cancelErr := client.CancelMarketOrdersByFilter(tt.params)
			if cancelErr != nil {
				t.Fatalf("cancel filter: %v", cancelErr)
			}
			if response == nil || response.Canceled == nil || response.NotCanceled == nil {
				t.Fatalf("response not normalized: %#v", response)
			}
			if len(response.Canceled) != 0 || len(response.NotCanceled) != 0 {
				t.Fatalf("safety invariant: expected no affected orders, got %#v", response)
			}
			t.Logf("production accepted %s filter with empty typed result", tt.name)
		})
	}
}
