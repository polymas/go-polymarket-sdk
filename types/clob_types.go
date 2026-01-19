package types

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ApiCreds 表示API凭证
type ApiCreds struct {
	Key        string `json:"apiKey"`
	Secret     string `json:"secret"`
	Passphrase string `json:"passphrase"`
}

// ClearApiCreds 清理敏感信息（API凭证）
// 安全建议：在不再需要 ApiCreds 时调用此函数清理内存中的敏感信息
// 注意：清理后 ApiCreds 将无法再使用，请确保在清理前不再需要API凭证
func ClearApiCreds(creds *ApiCreds) {
	if creds == nil {
		return
	}
	creds.Key = ""
	creds.Secret = ""
	creds.Passphrase = ""
	// 注意：Go的垃圾回收器会处理字符串的内存清理
	// 但显式清零可以更快地释放内存引用
}

// RequestBody 表示用于HMAC签名的请求体
// 它是一个JSON字符串（json.RawMessage），保留原始结构体的字段顺序
//
// IMPORTANT: To preserve field order for HMAC signature matching with Python:
//   - Body should be a JSON string (json.RawMessage) from json.Marshal of a struct
//   - Do NOT use map[string]interface{} as Go's json.Marshal will sort map keys alphabetically
//   - Structs maintain field order when marshaled, matching Python's dict insertion order
type RequestBody json.RawMessage

// RequestArgs 表示用于签名的请求参数
type RequestArgs struct {
	Method      string       `json:"method"` // GET, POST, DELETE
	RequestPath string       `json:"request_path"`
	Body        *RequestBody `json:"body,omitempty"` // nil means no body
}

// TokenValue 表示带值的代币
type TokenValue struct {
	TokenID string  `json:"token_id"`
	Value   float64 `json:"value"`
}

// Midpoint 表示中间市场价格
type Midpoint TokenValue

// Spread 表示价差值
type Spread TokenValue

// BookParams 表示订单簿查询的参数
type BookParams struct {
	TokenID string `json:"token_id"`
	Side    string `json:"side"` // BUY or SELL
}

// Price 表示代币和方向的价格
type Price struct {
	BookParams
	Price float64 `json:"price"`
}

// UnmarshalJSON 实现Price的自定义JSON反序列化，处理price可能是字符串或数字的情况
func (p *Price) UnmarshalJSON(data []byte) error {
	// 使用临时结构体来解析JSON
	var temp struct {
		TokenID string      `json:"token_id"`
		Side    string      `json:"side"`
		Price   interface{} `json:"price"` // 可能是数字或字符串
	}

	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	// 复制字段
	p.TokenID = temp.TokenID
	p.Side = temp.Side

	// 处理price（可能是数字或字符串）
	switch v := temp.Price.(type) {
	case float64:
		p.Price = v
	case int:
		p.Price = float64(v)
	case int64:
		p.Price = float64(v)
	case string:
		// 尝试解析为浮点数
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			p.Price = parsed
		} else {
			return fmt.Errorf("failed to parse price as float: %w", err)
		}
	default:
		return fmt.Errorf("unexpected price type: %T", v)
	}

	return nil
}

// BidAsk 表示买卖价格
type BidAsk struct {
	BUY  *float64 `json:"BUY,omitempty"`
	SELL *float64 `json:"SELL,omitempty"`
}

// TokenBidAsk 表示特定代币的买卖价格
type TokenBidAsk struct {
	BidAsk
	TokenID string `json:"token_id"`
}

// Token 表示市场代币
type Token struct {
	TokenID string  `json:"token_id"`
	Outcome string  `json:"outcome"`
	Price   float64 `json:"price"`
	Winner  *bool   `json:"winner,omitempty"`
}

// PriceHistory 表示代币的价格历史
type PriceHistory struct {
	TokenID string            `json:"token_id"`
	History []TimeseriesPoint `json:"history"`
}

// PaginatedResponse 表示分页API响应
type PaginatedResponse[T any] struct {
	Data       []T    `json:"data"`
	NextCursor string `json:"next_cursor"`
	Limit      int    `json:"limit"`
	Count      int    `json:"count"`
}

// OrderType 表示订单类型
type OrderType string

const (
	OrderTypeGTC OrderType = "GTC" // Good Till Cancel
	OrderTypeIOC OrderType = "IOC" // Immediate Or Cancel
	OrderTypeFOK OrderType = "FOK" // Fill Or Kill
)

// OrderSide 表示订单方向
type OrderSide string

const (
	OrderSideBUY  OrderSide = "BUY"
	OrderSideSELL OrderSide = "SELL"
)

// OrderArgs 表示创建订单的参数
type OrderArgs struct {
	TokenID    string    `json:"token_id"`
	Price      float64   `json:"price"`
	Size       float64   `json:"size"`
	Side       OrderSide `json:"side"`
	FeeRateBps *int      `json:"fee_rate_bps,omitempty"`
}

// MarketOrderArgs 表示创建市价单的参数
type MarketOrderArgs struct {
	OrderArgs
	Amount    float64   `json:"amount"`
	OrderType OrderType `json:"order_type"`
}

// FloatString 处理可能来自API的字符串格式的浮点值
type FloatString float64

// UnmarshalJSON 实现FloatString的自定义JSON反序列化
func (fs *FloatString) UnmarshalJSON(data []byte) error {
	// Try parsing as float first
	var f float64
	if err := json.Unmarshal(data, &f); err == nil {
		*fs = FloatString(f)
		return nil
	}

	// Try parsing as string
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	// Handle empty string or "0"
	if s == "" || s == "0" {
		*fs = FloatString(0)
		return nil
	}

	// Parse string as float
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		*fs = FloatString(f)
		return nil
	}

	return fmt.Errorf("cannot parse %q as float", s)
}

// MarshalJSON 实现FloatString的自定义JSON序列化
func (fs FloatString) MarshalJSON() ([]byte, error) {
	return json.Marshal(float64(fs))
}

// Float64 返回float64值
func (fs FloatString) Float64() float64 {
	return float64(fs)
}

// NullableTime 包装*time.Time以处理JSON中的"0"或空字符串
type NullableTime struct {
	*time.Time
}

// UnmarshalJSON 实现NullableTime的自定义JSON反序列化
func (nt *NullableTime) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		// If unmarshaling as string fails, try as number (timestamp)
		var ts int64
		if err2 := json.Unmarshal(data, &ts); err2 == nil {
			if ts == 0 {
				nt.Time = nil // nil for zero/invalid time
				return nil
			}
			t := time.Unix(ts, 0)
			nt.Time = &t
			return nil
		}
		return err
	}

	// Handle "0" or empty string - set to nil
	if s == "" || s == "0" {
		nt.Time = nil
		return nil
	}

	// Try parsing as RFC3339 first (full datetime)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		nt.Time = &t
		return nil
	}

	// Try parsing as Unix timestamp (string format)
	if strings.HasPrefix(s, "1") && len(s) == 10 {
		// Looks like a Unix timestamp string
		var ts int64
		if _, err := fmt.Sscanf(s, "%d", &ts); err == nil {
			t := time.Unix(ts, 0)
			nt.Time = &t
			return nil
		}
	}

	// Try parsing as date-only format (2006-01-02)
	if t, err := time.Parse("2006-01-02", s); err == nil {
		nt.Time = &t
		return nil
	}

	// Try parsing as date with time but no timezone (2006-01-02T15:04:05)
	if strings.Contains(s, "T") && !strings.Contains(s, "Z") && !strings.Contains(s, "+") && !strings.Contains(s, "-") {
		if t, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
			nt.Time = &t
			return nil
		}
	}

	// If all parsing fails, set to nil
	nt.Time = nil
	return nil
}

// MarshalJSON 实现NullableTime的自定义JSON序列化
func (nt NullableTime) MarshalJSON() ([]byte, error) {
	if nt.Time == nil {
		return json.Marshal("0")
	}
	return json.Marshal(nt.Time.Format(time.RFC3339))
}

// OpenOrder 表示开放订单
type OpenOrder struct {
	OrderID         Keccak256    `json:"id"` // alias: id
	Status          string       `json:"status"`
	Owner           string       `json:"owner"`
	MakerAddress    string       `json:"maker_address"`
	ConditionID     Keccak256    `json:"market"`   // alias: market
	TokenID         string       `json:"asset_id"` // alias: asset_id
	Side            OrderSide    `json:"side"`
	OriginalSize    FloatString  `json:"original_size"`
	SizeMatched     FloatString  `json:"size_matched"` // alias: size_matched (not filled_size)
	Price           FloatString  `json:"price"`
	Outcome         string       `json:"outcome"`
	Expiration      NullableTime `json:"expiration"`
	OrderType       string       `json:"order_type"` // GTC or GTD
	AssociateTrades []string     `json:"associate_trades"`
	CreatedAt       NullableTime `json:"created_at"`
}

// OrderPostResponse 表示提交订单的响应
// API返回camelCase格式：errorMsg, orderID
type OrderPostResponse struct {
	OrderID  Keccak256 `json:"orderID"`
	Status   string    `json:"status"`
	ErrorMsg string    `json:"errorMsg"`
}

// OrderCancelResponse 表示取消订单的响应
type OrderCancelResponse struct {
	Canceled    []Keccak256          `json:"canceled,omitempty"`
	NotCanceled map[Keccak256]string `json:"not_canceled,omitempty"`
}

// OrderBookSummary 表示订单簿摘要
type OrderBookSummary struct {
	TokenID string       `json:"token_id"`
	Bids    []OrderLevel `json:"bids,omitempty"`
	Asks    []OrderLevel `json:"asks,omitempty"`
}

// OrderLevel 表示订单簿中的价格层级
// price和size使用FloatString类型，可自动处理JSON中的数字或字符串格式
type OrderLevel struct {
	Price FloatString `json:"price"` // 价格（支持数字或字符串格式）
	Size  FloatString `json:"size"`  // 数量（支持数字或字符串格式）
}

// OrderBookSummaryResponse 表示批量获取订单簿API的响应
// 根据 POST /books API 文档：https://docs.polymarket.com/api-reference/orderbook/get-multiple-order-books-summaries-by-request
type OrderBookSummaryResponse struct {
	AssetID      string       `json:"asset_id"`       // 资产ID
	Market       string       `json:"market"`         // 市场ID（condition_id）
	Timestamp    string       `json:"timestamp"`      // 时间戳
	Hash         string       `json:"hash"`           // 订单簿哈希
	MinOrderSize string       `json:"min_order_size"` // 最小订单大小
	TickSize     string       `json:"tick_size"`      // tick大小
	Bids         []OrderLevel `json:"bids,omitempty"` // 买盘
	Asks         []OrderLevel `json:"asks,omitempty"` // 卖盘
}

// ClobMarket 表示CLOB市场
type ClobMarket struct {
	TokenIDs                []Token    `json:"tokens"`
	ConditionID             Keccak256  `json:"condition_id"`
	QuestionID              Keccak256  `json:"question_id"`
	Question                string     `json:"question"`
	Description             string     `json:"description"`
	MarketSlug              string     `json:"market_slug"`
	EndDateISO              *time.Time `json:"end_date_iso,omitempty"`
	GameStartTime           *time.Time `json:"game_start_time,omitempty"`
	SecondsDelay            int        `json:"seconds_delay"`
	EnableOrderBook         bool       `json:"enable_order_book"`
	AcceptingOrders         bool       `json:"accepting_orders"`
	AcceptingOrderTimestamp *time.Time `json:"accepting_order_timestamp,omitempty"`
	MinimumOrderSize        float64    `json:"minimum_order_size"`
	MinimumTickSize         float64    `json:"minimum_tick_size"`
	Active                  bool       `json:"active"`
	Closed                  bool       `json:"closed"`
	Archived                bool       `json:"archived"`
	NegRisk                 bool       `json:"neg_risk"`
	NegRiskMarketID         Keccak256  `json:"neg_risk_market_id"`
}

// TickSize 表示tick大小值
type TickSize string

// RewardRate 表示奖励费率
type RewardRate struct {
	AssetAddress     EthAddress `json:"asset_address"`
	RewardsDailyRate float64    `json:"rewards_daily_rate"`
}

// RewardConfig 表示奖励配置
type RewardConfig struct {
	AssetAddress     EthAddress `json:"asset_address"`
	RewardsDailyRate float64    `json:"rate_per_day"`
	StartDate        time.Time  `json:"start_date"`
	EndDate          time.Time  `json:"end_date"`
	RewardID         *string    `json:"id,omitempty"`
	TotalRewards     float64    `json:"total_rewards"`
	TotalDays        *int       `json:"total_days,omitempty"`
}

// Rewards 表示奖励信息
type Rewards struct {
	Rates            []RewardRate `json:"rates,omitempty"`
	RewardsMinSize   int          `json:"min_size"`
	RewardsMaxSpread float64      `json:"max_spread"`
}

// EarnedReward 表示已获得的奖励
type EarnedReward struct {
	AssetAddress EthAddress `json:"asset_address"`
	Earnings     float64    `json:"earnings"`
	AssetRate    float64    `json:"asset_rate"`
}

// DailyEarnedReward 表示每日获得的奖励
type DailyEarnedReward struct {
	Date         time.Time  `json:"date"`
	AssetAddress EthAddress `json:"asset_address"`
	MakerAddress EthAddress `json:"maker_address"`
	Earnings     float64    `json:"earnings"`
	AssetRate    float64    `json:"asset_rate"`
}

// RewardMarket 表示带奖励的市场
type RewardMarket struct {
	MarketID              string         `json:"market_id"`
	ConditionID           Keccak256      `json:"condition_id"`
	Question              string         `json:"question"`
	MarketSlug            string         `json:"market_slug"`
	EventSlug             string         `json:"event_slug"`
	Image                 string         `json:"image"`
	MakerAddress          EthAddress     `json:"maker_address"`
	Tokens                []Token        `json:"tokens"`
	RewardsConfig         []RewardConfig `json:"rewards_config"`
	Earnings              []EarnedReward `json:"earnings"`
	RewardsMaxSpread      float64        `json:"rewards_max_spread"`
	RewardsMinSize        float64        `json:"rewards_min_size"`
	EarningPercentage     float64        `json:"earning_percentage"`
	Spread                float64        `json:"spread"`
	MarketCompetitiveness float64        `json:"market_competitiveness"`
}

// MarketRewards 表示市场奖励信息
type MarketRewards struct {
	ConditionID           Keccak256      `json:"condition_id"`
	Question              string         `json:"question"`
	MarketSlug            string         `json:"market_slug"`
	EventSlug             string         `json:"event_slug"`
	Image                 string         `json:"image"`
	Tokens                []Token        `json:"tokens"`
	RewardsConfig         []RewardConfig `json:"rewards_config"`
	RewardsMaxSpread      float64        `json:"rewards_max_spread"`
	RewardsMinSize        int            `json:"rewards_min_size"`
	MarketCompetitiveness float64        `json:"market_competitiveness"`
}

// PolygonTrade 表示Polygon上的交易
type PolygonTrade struct {
	TradeID      string     `json:"id"`
	ConditionID  Keccak256  `json:"market"`
	TokenID      string     `json:"asset_id"`
	Side         OrderSide  `json:"side"`
	Price        float64    `json:"price"`
	Size         float64    `json:"size"`
	Timestamp    time.Time  `json:"timestamp"`
	MakerAddress EthAddress `json:"maker_address"`
	TakerAddress EthAddress `json:"taker_address"`
}

// LastTradePrice 表示最后成交价
type LastTradePrice TokenValue

// BalanceAllowance 表示余额授权信息
type BalanceAllowance struct {
	Allowance float64 `json:"allowance"`
	Balance   float64 `json:"balance"`
}

// APIKey 表示 API 密钥信息
type APIKey struct {
	ID        string    `json:"id"`
	Key       string    `json:"key"`
	CreatedAt time.Time `json:"created_at"`
	Readonly  bool      `json:"readonly,omitempty"`
}

// Notification 表示通知信息
type Notification struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}

// RFQRequest 表示 RFQ 请求
type RFQRequest struct {
	TokenID string    `json:"token_id"`
	Side    OrderSide `json:"side"` // BUY or SELL
	Size    float64   `json:"size"`
	Price   *float64  `json:"price,omitempty"` // Optional: desired price
}

// RFQResponse 表示 RFQ 请求响应
type RFQResponse struct {
	RequestID string    `json:"request_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// RFQQuote 表示 RFQ 报价
type RFQQuote struct {
	QuoteID   string    `json:"quote_id"`
	RequestID string    `json:"request_id"`
	TokenID   string    `json:"token_id"`
	Side      OrderSide `json:"side"`
	Price     float64   `json:"price"`
	Size      float64   `json:"size"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// RFQAcceptResponse 表示接受报价的响应
type RFQAcceptResponse struct {
	QuoteID  string    `json:"quote_id"`
	OrderID  Keccak256 `json:"order_id,omitempty"`
	Status   string    `json:"status"`
	AcceptedAt time.Time `json:"accepted_at"`
}
