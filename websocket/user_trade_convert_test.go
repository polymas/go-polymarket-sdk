package websocket

import (
	"testing"

	"github.com/polymas/go-polymarket-sdk/types"
)

func TestUserTradeEventToClobTrade(t *testing.T) {
	ev := &UserTradeEvent{
		ID: "trade-1", TakerOrderID: "0xabc", Side: types.OrderSideBUY, Size: "10", Price: "0.5",
		Status: "CONFIRMED", TransactionHash: "0xdef", TraderSide: "TAKER",
		MakerOrders: []UserTradeMakerOrder{{OrderID: "0x111", MatchedAmount: "10", Price: "0.5", Side: types.OrderSideSELL}},
	}
	got := ev.ToClobTrade()
	if got.ID != "trade-1" || got.TakerOrderID != "0xabc" || got.Status != "CONFIRMED" || got.TransactionHash != "0xdef" {
		t.Fatalf("top-level fields not mapped: %+v", got)
	}
	if len(got.MakerOrders) != 1 || got.MakerOrders[0].OrderID != "0x111" || got.MakerOrders[0].Side != types.OrderSideSELL {
		t.Fatalf("maker orders not mapped: %+v", got.MakerOrders)
	}
}
