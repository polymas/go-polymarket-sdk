package types

import (
	"encoding/json"
	"testing"
)

func TestNormalizeOrderStatus(t *testing.T) {
	tests := []struct {
		raw   string
		want  OrderStatus
		known bool
	}{
		{"live", OrderStatusLive, true},
		{"LIVE", OrderStatusLive, true},
		{"ORDER_STATUS_LIVE", OrderStatusLive, true},
		{"order_status_matched", OrderStatusMatched, true},
		{"ORDER_STATUS_CANCELED_MARKET_RESOLVED", OrderStatusCanceledMarketResolved, true},
		{"CANCELLED", OrderStatusCanceled, true},
		{"Partially_Filled", OrderStatusPartiallyFilled, true},
		{"ORDER_STATUS_FUTURE", OrderStatus("future"), false},
	}
	for _, tt := range tests {
		got := NormalizeOrderStatus(tt.raw)
		if got != tt.want || got.Known() != tt.known {
			t.Errorf("NormalizeOrderStatus(%q)=%q known=%v, want %q known=%v", tt.raw, got, got.Known(), tt.want, tt.known)
		}
	}
	if !OrderStatusLive.IsOpen() || OrderStatusDelayed.IsOpen() || !OrderStatusCanceled.IsTerminal() {
		t.Fatal("order status classification mismatch")
	}
}

func TestOrderStatusPreservesWireValue(t *testing.T) {
	var order OpenOrder
	if err := json.Unmarshal([]byte(`{"status":"ORDER_STATUS_LIVE"}`), &order); err != nil {
		t.Fatal(err)
	}
	if order.Status != "ORDER_STATUS_LIVE" || order.RawStatus != "ORDER_STATUS_LIVE" || order.NormalizedStatus() != OrderStatusLive || !order.IsOpen() {
		t.Fatalf("order=%+v normalized=%q", order, order.NormalizedStatus())
	}

	var response OrderPostResponse
	if err := json.Unmarshal([]byte(`{"status":"MATCHED","success":true}`), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "MATCHED" || response.RawStatus != "MATCHED" || response.NormalizedStatus() != OrderStatusMatched || !response.Accepted() {
		t.Fatalf("response=%+v normalized=%q", response, response.NormalizedStatus())
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" || string(encoded) == "null" {
		t.Fatalf("encoded=%s", encoded)
	}
}
