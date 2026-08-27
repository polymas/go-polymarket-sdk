package clob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

const (
	testPrivateKey = "4f3edf983ac63ad7c696ac528e95d8cfe10b8a6a6b65e6f4f5f3f0f5d7f6f7a8"
	testCondition  = "0x1111111111111111111111111111111111111111111111111111111111111111"
	testOrderID    = "0x2222222222222222222222222222222222222222222222222222222222222222"
	testTxHash     = "0x3333333333333333333333333333333333333333333333333333333333333333"
	testTokenID    = "42"
)

func newAuthenticatedOrderClientForTest(t *testing.T, baseURL string) *orderClientImpl {
	t.Helper()
	web3Client, err := web3.NewClient(testPrivateKey, types.EOASignatureType, types.Polygon, baseURL)
	if err != nil {
		t.Fatalf("create web3 test client: %v", err)
	}
	t.Cleanup(web3Client.Close)
	return &orderClientImpl{baseClient: &baseClient{
		baseURL:    baseURL,
		web3Client: web3Client,
		deriveCreds: &types.ApiCreds{
			Key:        "test-key",
			Secret:     "dGVzdC1zZWNyZXQ=",
			Passphrase: "test-passphrase",
		},
	}}
}

func TestGetTradesUsesOfficialFiltersAndPagination(t *testing.T) {
	conditionID, _ := types.ParseConditionID(testCondition)
	tokenID, _ := types.ParseTokenID(testTokenID)
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != internal.Trades {
			t.Fatalf("path = %q, want %q", r.URL.Path, internal.Trades)
		}
		if r.Header.Get(internal.PolyAPIKey) != "test-key" {
			t.Fatalf("missing L2 authentication headers")
		}
		if r.URL.Query().Get("market") != testCondition || r.URL.Query().Get("asset_id") != testTokenID ||
			r.URL.Query().Get("after") != "100" || r.URL.Query().Get("before") != "200" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}

		call := calls.Add(1)
		nextCursor := internal.EndCursor
		if call == 1 {
			nextCursor = "MQ=="
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"limit":       1,
			"next_cursor": nextCursor,
			"count":       1,
			"data": []map[string]interface{}{{
				"id":               fmt.Sprintf("trade-%d", call),
				"taker_order_id":   testOrderID,
				"market":           testCondition,
				"asset_id":         testTokenID,
				"side":             "BUY",
				"size":             "5.000000",
				"fee_rate_bps":     "0",
				"price":            "0.500000",
				"status":           "TRADE_STATUS_CONFIRMED",
				"match_time":       "1700000000",
				"last_update":      "1700000001",
				"outcome":          "YES",
				"bucket_index":     0,
				"owner":            "owner",
				"maker_address":    "0x4444444444444444444444444444444444444444",
				"transaction_hash": testTxHash,
				"trader_side":      "TAKER",
				"maker_orders":     []interface{}{},
			}},
		})
	}))
	defer server.Close()

	client := newAuthenticatedOrderClientForTest(t, server.URL)
	trades, err := client.GetTrades(&types.ClobTradeParams{
		Market:  conditionID,
		AssetID: tokenID,
		After:   100,
		Before:  200,
	})
	if err != nil {
		t.Fatalf("GetTrades: %v", err)
	}
	if len(trades) != 2 || trades[0].ID != "trade-1" || trades[1].ID != "trade-2" {
		t.Fatalf("trades = %+v", trades)
	}
	if trades[0].Market.String() != testCondition || trades[0].AssetID.String() != testTokenID || trades[0].Price != "0.500000" {
		t.Fatalf("trade precision/IDs were not preserved: %+v", trades[0])
	}
}

func TestGetOrderUsesSingleOrderEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != internal.GetOrder+testOrderID {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":       testOrderID,
			"status":   "matched",
			"market":   testCondition,
			"asset_id": testTokenID,
			"side":     "BUY",
			"price":    "0.5",
		})
	}))
	defer server.Close()

	client := newAuthenticatedOrderClientForTest(t, server.URL)
	order, err := client.GetOrder(types.Keccak256(testOrderID))
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if order.OrderID.String() != testOrderID || order.TokenID != testTokenID {
		t.Fatalf("order = %+v", order)
	}
}

func TestWaitForOrderFillSettlementHandlesMinedAndFailedTrades(t *testing.T) {
	var tradeACalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		status := "FAILED"
		transactionHash := ""
		if id == "trade-a" {
			status = "MATCHED"
			if tradeACalls.Add(1) >= 2 {
				status = "TRADE_STATUS_MINED"
				transactionHash = testTxHash
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"next_cursor": internal.EndCursor,
			"data": []map[string]interface{}{{
				"id": id, "taker_order_id": testOrderID, "market": testCondition,
				"asset_id": testTokenID, "side": "BUY", "size": "5", "price": "0.5",
				"status": status, "transaction_hash": transactionHash,
			}},
		})
	}))
	defer server.Close()

	client := newAuthenticatedOrderClientForTest(t, server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	report, err := client.WaitForOrderFillSettlement(ctx, types.OrderPostResponse{
		Status:   "matched",
		TradeIDs: []string{"trade-a", "trade-b"},
	})
	if err != nil {
		t.Fatalf("WaitForOrderFillSettlement: %v", err)
	}
	if report.State != types.OrderSettlementFailed {
		t.Fatalf("state = %q, want failed", report.State)
	}
	if len(report.TransactionsHashes) != 1 || report.TransactionsHashes[0].String() != testTxHash {
		t.Fatalf("transaction hashes = %v", report.TransactionsHashes)
	}
	if len(report.Trades) != 2 || report.Timing.PollCount < 2 || report.Timing.QueryErrors != 0 {
		t.Fatalf("report = %+v", report)
	}
}

func TestWaitForOrderFillSettlementReportsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"next_cursor": internal.EndCursor,
			"data":        []interface{}{},
		})
	}))
	defer server.Close()

	client := newAuthenticatedOrderClientForTest(t, server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	report, err := client.WaitForOrderFillSettlement(ctx, types.OrderPostResponse{TradeIDs: []string{"pending"}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	if report.State != types.OrderSettlementTimeout || !report.Timing.SettlementTimedOut {
		t.Fatalf("report = %+v, want timeout", report)
	}
}

func TestResolveOrderTransactionsUsesOneBatchDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"next_cursor": internal.EndCursor,
			"data": []map[string]interface{}{{
				"id": id, "taker_order_id": testOrderID, "market": testCondition,
				"asset_id": testTokenID, "status": "MINED", "transaction_hash": testTxHash,
			}},
		})
	}))
	defer server.Close()

	client := newAuthenticatedOrderClientForTest(t, server.URL)
	responses := []types.OrderPostResponse{
		{Status: "matched", TradeIDs: []string{"trade-a"}},
		{Status: "matched", TradeIDs: []string{"trade-b"}},
	}
	client.resolveOrderTransactions(responses, time.Second)
	for i, response := range responses {
		if response.SettlementState != types.OrderSettlementMined || len(response.TransactionsHashes) != 1 {
			t.Fatalf("response[%d] = %+v", i, response)
		}
		if response.Timing.PollCount != 1 || response.Timing.SettlementWaitDuration <= 0 {
			t.Fatalf("response[%d] timing = %+v", i, response.Timing)
		}
	}
}

func TestResolveOrderTransactionsTimeoutIsPerOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		data := []map[string]interface{}{}
		if id == "trade-resolved" {
			data = append(data, map[string]interface{}{
				"id": id, "taker_order_id": testOrderID, "market": testCondition,
				"asset_id": testTokenID, "status": "MINED", "transaction_hash": testTxHash,
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"next_cursor": internal.EndCursor,
			"data":        data,
		})
	}))
	defer server.Close()

	client := newAuthenticatedOrderClientForTest(t, server.URL)
	responses := []types.OrderPostResponse{
		{Status: "matched", TradeIDs: []string{"trade-resolved"}},
		{Status: "matched", TradeIDs: []string{"trade-pending"}},
	}
	client.resolveOrderTransactions(responses, 15*time.Millisecond)
	if responses[0].SettlementState != types.OrderSettlementMined || responses[0].Timing.SettlementTimedOut {
		t.Fatalf("resolved response = %+v", responses[0])
	}
	if responses[1].SettlementState != types.OrderSettlementTimeout || !responses[1].Timing.SettlementTimedOut {
		t.Fatalf("pending response = %+v", responses[1])
	}
}

func TestReconcileExpectedOrdersPollsDeterministicOrderHash(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":               testOrderID,
			"status":           "matched",
			"market":           testCondition,
			"asset_id":         testTokenID,
			"associate_trades": []string{"trade-a"},
		})
	}))
	defer server.Close()

	client := newAuthenticatedOrderClientForTest(t, server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	orders, timing, err := client.reconcileExpectedOrders(
		ctx,
		[]types.Keccak256{types.Keccak256(testOrderID)},
		time.Millisecond,
	)
	if err != nil {
		t.Fatalf("reconcileExpectedOrders: %v", err)
	}
	if len(orders) != 1 || orders[types.Keccak256(testOrderID)].Status != "matched" {
		t.Fatalf("orders = %+v", orders)
	}
	if !timing.Reconciled || timing.ReconciliationPollCount < 2 || timing.ReconciliationQueryErrors != 1 || timing.ReconciliationWaitDuration <= 0 {
		t.Fatalf("timing = %+v", timing)
	}
}

func TestReconcileAmbiguousPostRestoresAcceptedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":               testOrderID,
			"status":           "ORDER_STATUS_MATCHED",
			"market":           testCondition,
			"asset_id":         testTokenID,
			"associate_trades": []string{},
		})
	}))
	defer server.Close()

	client := newAuthenticatedOrderClientForTest(t, server.URL)
	responses, complete := client.reconcileAmbiguousPost(
		[]types.Keccak256{types.Keccak256(testOrderID)},
		[]orderRequestV2{{Order: orderedOrderV2{MakerAmount: "5000000", TakerAmount: "2500000"}}},
		time.Now(),
		10*time.Millisecond,
	)
	if !complete || len(responses) != 1 {
		t.Fatalf("complete=%v responses=%+v", complete, responses)
	}
	response := responses[0]
	if !response.Success || response.Status != "matched" || response.OrderID.String() != testOrderID ||
		response.MakingAmount != "5000000" || response.TakingAmount != "2500000" || !response.Timing.Reconciled {
		t.Fatalf("response = %+v", response)
	}
}

func TestOrderPostResponseModelsOfficialAsyncFields(t *testing.T) {
	var response types.OrderPostResponse
	err := json.Unmarshal([]byte(`{
		"success":true,"orderID":"`+testOrderID+`","status":"matched",
		"makingAmount":"5000000","takingAmount":"2500000",
		"transactionsHashes":["`+testTxHash+`"],"tradeIDs":["trade-a"],"errorMsg":""
	}`), &response)
	if err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !response.Success || response.MakingAmount != "5000000" || response.TakingAmount != "2500000" ||
		len(response.TransactionsHashes) != 1 || len(response.TradeIDs) != 1 {
		t.Fatalf("response = %+v", response)
	}
}

func TestAwaitOrderResultsReconcilesUnknownThenSettlement(t *testing.T) {
	var orderCalls atomic.Int32
	var tradeCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == internal.GetOrder+testOrderID:
			orderCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":               testOrderID,
				"status":           "ORDER_STATUS_MATCHED",
				"market":           testCondition,
				"asset_id":         testTokenID,
				"associate_trades": []string{"trade-a"},
			})
		case r.URL.Path == internal.Trades:
			tradeCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"next_cursor": internal.EndCursor,
				"data": []map[string]interface{}{{
					"id": "trade-a", "taker_order_id": testOrderID,
					"market": testCondition, "asset_id": testTokenID,
					"status": "MINED", "transaction_hash": testTxHash,
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newAuthenticatedOrderClientForTest(t, server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	responses, err := client.AwaitOrderResults(ctx, []types.OrderPostResponse{{
		Status:          OrderUnknownStatus,
		ExpectedOrderID: types.Keccak256(testOrderID),
		MakingAmount:    "5000000",
		TakingAmount:    "50000",
	}})
	if err != nil {
		t.Fatalf("AwaitOrderResults: %v", err)
	}
	if len(responses) != 1 {
		t.Fatalf("responses = %+v", responses)
	}
	response := responses[0]
	if !response.Accepted() || response.Status != "matched" || response.OrderID.String() != testOrderID {
		t.Fatalf("response = %+v", response)
	}
	if response.SettlementState != types.OrderSettlementMined || len(response.TransactionsHashes) != 1 {
		t.Fatalf("settlement was not completed: %+v", response)
	}
	if response.MakingAmount != "5000000" || response.TakingAmount != "50000" ||
		!response.Timing.Reconciled || response.Timing.ReconciliationPollCount != 1 ||
		response.Timing.PollCount != 1 || response.Timing.TotalDuration <= 0 {
		t.Fatalf("metadata/timing = %+v", response)
	}
	if orderCalls.Load() != 1 || tradeCalls.Load() != 1 {
		t.Fatalf("order calls=%d trade calls=%d", orderCalls.Load(), tradeCalls.Load())
	}
}

func TestAwaitOrderResultsUsesOneBatchDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"next_cursor": internal.EndCursor,
			"data":        []interface{}{},
		})
	}))
	defer server.Close()

	client := newAuthenticatedOrderClientForTest(t, server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	responses, err := client.AwaitOrderResults(ctx, []types.OrderPostResponse{
		{Status: "matched", TradeIDs: []string{"trade-a"}},
		{Status: "matched", TradeIDs: []string{"trade-b"}},
	})
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	if elapsed > 150*time.Millisecond {
		t.Fatalf("batch wait took %v; deadline appears to have been applied per order", elapsed)
	}
	for i, response := range responses {
		if response.SettlementState != types.OrderSettlementTimeout || !response.Timing.SettlementTimedOut {
			t.Fatalf("response[%d] = %+v", i, response)
		}
	}
}

func TestAwaitOrderResultsReturnsTerminalResultsWithoutNetwork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newAuthenticatedOrderClientForTest(t, server.URL)
	responses, err := client.AwaitOrderResults(context.Background(), []types.OrderPostResponse{
		{Success: true, Status: "live", OrderID: types.Keccak256(testOrderID)},
		{Status: OrderMarketClosedStatus, ErrorMsg: "closed"},
	})
	if err != nil {
		t.Fatalf("AwaitOrderResults: %v", err)
	}
	if len(responses) != 2 || !responses[0].Accepted() || !responses[1].DefinitelyNotSubmitted() {
		t.Fatalf("responses = %+v", responses)
	}
	if calls.Load() != 0 {
		t.Fatalf("terminal responses caused %d network calls", calls.Load())
	}
}

func TestOrderPostResponseStateHelpers(t *testing.T) {
	tests := []struct {
		name         string
		response     types.OrderPostResponse
		accepted     bool
		followUp     bool
		notSubmitted bool
	}{
		{name: "live", response: types.OrderPostResponse{Status: "live"}, accepted: true},
		{name: "matched pending", response: types.OrderPostResponse{Status: "matched", TradeIDs: []string{"trade"}}, accepted: true, followUp: true},
		{name: "unknown", response: types.OrderPostResponse{Status: OrderUnknownStatus}, followUp: true},
		{name: "market closed", response: types.OrderPostResponse{Status: OrderMarketClosedStatus}, notSubmitted: true},
		{name: "server rejected", response: types.OrderPostResponse{Status: OrderServerRejectedStatus}, notSubmitted: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.response.Accepted(); got != test.accepted {
				t.Fatalf("Accepted() = %v, want %v", got, test.accepted)
			}
			if got := test.response.NeedsFollowUp(); got != test.followUp {
				t.Fatalf("NeedsFollowUp() = %v, want %v", got, test.followUp)
			}
			if got := test.response.DefinitelyNotSubmitted(); got != test.notSubmitted {
				t.Fatalf("DefinitelyNotSubmitted() = %v, want %v", got, test.notSubmitted)
			}
		})
	}
}
