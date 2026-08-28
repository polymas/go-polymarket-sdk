package websocket

import "github.com/polymas/go-polymarket-sdk/types"

// UserOrderEvent is an order placement, update, or cancellation from the
// authenticated User Channel. Decimal values and timestamps remain strings to
// preserve the exact values sent by Polymarket.
type UserOrderEvent struct {
	EventType       string            `json:"event_type"`
	ID              string            `json:"id"`
	Owner           string            `json:"owner"`
	Market          types.ConditionID `json:"market"`
	TokenID         types.TokenID     `json:"asset_id"`
	Side            types.OrderSide   `json:"side"`
	OrderOwner      string            `json:"order_owner"`
	OriginalSize    string            `json:"original_size"`
	SizeMatched     string            `json:"size_matched"`
	Price           string            `json:"price"`
	AssociateTrades []string          `json:"associate_trades"`
	Outcome         string            `json:"outcome"`
	Type            string            `json:"type"`
	CreatedAt       string            `json:"created_at"`
	Expiration      string            `json:"expiration"`
	OrderType       types.OrderType   `json:"order_type"`
	Status          string            `json:"status"`
	MakerAddress    string            `json:"maker_address"`
	Timestamp       string            `json:"timestamp"`
}

// UserTradeMakerOrder is one maker order included in a UserTradeEvent.
type UserTradeMakerOrder struct {
	OrderID       string          `json:"order_id"`
	Owner         string          `json:"owner"`
	MakerAddress  string          `json:"maker_address"`
	MatchedAmount string          `json:"matched_amount"`
	Price         string          `json:"price"`
	TokenID       types.TokenID   `json:"asset_id"`
	Outcome       string          `json:"outcome"`
	Side          types.OrderSide `json:"side"`
}

// UserTradeEvent is a trade lifecycle event from the authenticated User
// Channel. The ID and status are intentionally preserved for business-level
// deduplication across reconnects.
type UserTradeEvent struct {
	EventType       string                `json:"event_type"`
	Type            string                `json:"type"`
	ID              string                `json:"id"`
	TakerOrderID    string                `json:"taker_order_id"`
	Market          types.ConditionID     `json:"market"`
	TokenID         types.TokenID         `json:"asset_id"`
	Side            types.OrderSide       `json:"side"`
	Size            string                `json:"size"`
	Price           string                `json:"price"`
	Status          string                `json:"status"`
	MatchTime       string                `json:"match_time"`
	LastUpdate      string                `json:"last_update"`
	Outcome         string                `json:"outcome"`
	Owner           string                `json:"owner"`
	TradeOwner      string                `json:"trade_owner"`
	MakerAddress    string                `json:"maker_address"`
	TransactionHash string                `json:"transaction_hash"`
	BucketIndex     int                   `json:"bucket_index"`
	MakerOrders     []UserTradeMakerOrder `json:"maker_orders"`
	TraderSide      string                `json:"trader_side"`
	Timestamp       string                `json:"timestamp"`
}
