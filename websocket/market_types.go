package websocket

import "github.com/polymas/go-polymarket-sdk/types"

// MarketEventType identifies a Market Channel event.
type MarketEventType string

const (
	MarketEventBook           MarketEventType = "book"
	MarketEventPriceChange    MarketEventType = "price_change"
	MarketEventLastTradePrice MarketEventType = "last_trade_price"
	MarketEventTickSizeChange MarketEventType = "tick_size_change"
	MarketEventBestBidAsk     MarketEventType = "best_bid_ask"
	MarketEventNewMarket      MarketEventType = "new_market"
	MarketEventMarketResolved MarketEventType = "market_resolved"
)

// MarketEvent is implemented by every typed Market Channel event. Reconnects
// can repeat snapshots or events; callers should make state updates idempotent.
type MarketEvent interface {
	Type() MarketEventType
}

// MarketBookLevel is one aggregated orderbook level. Decimal strings are kept
// exactly as sent by Polymarket.
type MarketBookLevel struct {
	Price string `json:"price"`
	Size  string `json:"size"`
}

type MarketBookEvent struct {
	EventType MarketEventType   `json:"event_type"`
	AssetID   types.TokenID     `json:"asset_id"`
	Market    types.ConditionID `json:"market"`
	Bids      []MarketBookLevel `json:"bids"`
	Asks      []MarketBookLevel `json:"asks"`
	Timestamp string            `json:"timestamp"`
	Hash      string            `json:"hash"`
}

func (*MarketBookEvent) Type() MarketEventType { return MarketEventBook }

type MarketPriceChange struct {
	AssetID types.TokenID   `json:"asset_id"`
	Price   string          `json:"price"`
	Size    string          `json:"size"`
	Side    types.OrderSide `json:"side"`
	Hash    string          `json:"hash"`
	BestBid string          `json:"best_bid"`
	BestAsk string          `json:"best_ask"`
}

type MarketPriceChangeEvent struct {
	EventType    MarketEventType     `json:"event_type"`
	Market       types.ConditionID   `json:"market"`
	PriceChanges []MarketPriceChange `json:"price_changes"`
	Timestamp    string              `json:"timestamp"`
}

func (*MarketPriceChangeEvent) Type() MarketEventType { return MarketEventPriceChange }

type MarketLastTradePriceEvent struct {
	EventType       MarketEventType   `json:"event_type"`
	AssetID         types.TokenID     `json:"asset_id"`
	Market          types.ConditionID `json:"market"`
	Price           string            `json:"price"`
	Size            string            `json:"size"`
	FeeRateBps      string            `json:"fee_rate_bps"`
	Side            types.OrderSide   `json:"side"`
	Timestamp       string            `json:"timestamp"`
	TransactionHash string            `json:"transaction_hash"`
}

func (*MarketLastTradePriceEvent) Type() MarketEventType { return MarketEventLastTradePrice }

// MarketTickSizeChangeEvent exposes the new tick size without a REST request,
// allowing the business layer to update its order configuration cache.
type MarketTickSizeChangeEvent struct {
	EventType   MarketEventType   `json:"event_type"`
	AssetID     types.TokenID     `json:"asset_id"`
	Market      types.ConditionID `json:"market"`
	OldTickSize types.TickSize    `json:"old_tick_size"`
	NewTickSize types.TickSize    `json:"new_tick_size"`
	Timestamp   string            `json:"timestamp"`
}

func (*MarketTickSizeChangeEvent) Type() MarketEventType { return MarketEventTickSizeChange }

type MarketBestBidAskEvent struct {
	EventType MarketEventType   `json:"event_type"`
	Market    types.ConditionID `json:"market"`
	AssetID   types.TokenID     `json:"asset_id"`
	BestBid   string            `json:"best_bid"`
	BestAsk   string            `json:"best_ask"`
	Spread    string            `json:"spread"`
	Timestamp string            `json:"timestamp"`
}

func (*MarketBestBidAskEvent) Type() MarketEventType { return MarketEventBestBidAsk }

type MarketEventMessage struct {
	ID          string `json:"id"`
	Ticker      string `json:"ticker"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type MarketNewMarketEvent struct {
	EventType             MarketEventType    `json:"event_type"`
	ID                    string             `json:"id"`
	Question              string             `json:"question"`
	Market                types.ConditionID  `json:"market"`
	Slug                  string             `json:"slug"`
	Description           string             `json:"description"`
	AssetIDs              []types.TokenID    `json:"assets_ids"`
	Outcomes              []string           `json:"outcomes"`
	EventMessage          MarketEventMessage `json:"event_message"`
	Timestamp             string             `json:"timestamp"`
	Tags                  []string           `json:"tags"`
	ConditionID           types.ConditionID  `json:"condition_id"`
	Active                bool               `json:"active"`
	ClobTokenIDs          []types.TokenID    `json:"clob_token_ids"`
	SportsMarketType      string             `json:"sports_market_type"`
	Line                  string             `json:"line"`
	GameStartTime         string             `json:"game_start_time"`
	OrderPriceMinTickSize types.TickSize     `json:"order_price_min_tick_size"`
	GroupItemTitle        string             `json:"group_item_title"`
}

func (*MarketNewMarketEvent) Type() MarketEventType { return MarketEventNewMarket }

type MarketResolvedEvent struct {
	EventType      MarketEventType   `json:"event_type"`
	ID             string            `json:"id"`
	Market         types.ConditionID `json:"market"`
	AssetIDs       []types.TokenID   `json:"assets_ids"`
	WinningAssetID types.TokenID     `json:"winning_asset_id"`
	WinningOutcome string            `json:"winning_outcome"`
	Timestamp      string            `json:"timestamp"`
	Tags           []string          `json:"tags"`
}

func (*MarketResolvedEvent) Type() MarketEventType { return MarketEventMarketResolved }
