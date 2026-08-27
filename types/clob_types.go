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

// MarketPricePoint 表示 CLOB /prices-history 返回的单个价格点（API 字段名为 t/p）
// 参考: https://docs.polymarket.com/developers/CLOB/timeseries
type MarketPricePoint struct {
	T int64   `json:"t"` // Unix 时间戳（秒）
	P float64 `json:"p"` // 价格
}

// PricesHistoryResponse 表示 GET /prices-history 的响应
type PricesHistoryResponse struct {
	History []MarketPricePoint `json:"history"`
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
	OrderTypeFOK OrderType = "FOK" // Fill Or Kill
	OrderTypeGTD OrderType = "GTD" // Good Till Date
	OrderTypeFAK OrderType = "FAK" // Fill And Kill
)

// OrderSide 表示订单方向
type OrderSide string

const (
	OrderSideBUY  OrderSide = "BUY"
	OrderSideSELL OrderSide = "SELL"
)

// OrderArgs 表示创建订单的参数。
//
// V2 必填字段：
//
//   - TickSize : 该 token 当前的最小价格步长。SDK 不在下单热路径查询；
//     调用方可使用自己的默认值、Gamma/WS 数据或显式调用市场数据接口获取。
//
// 可选 V2 字段（零值即当前默认行为）：
//
//   - Expiration : GTD 订单过期 unix 秒；0 表示 GTC（不过期）
//   - PostOnly   : 只做 maker；下单瞬间会被 match 则拒单（不上单）
//   - DeferExec  : 静默入态机但不立即撮合；做市商批量预提交场景使用
//
// 当前订单路径统一使用这些 V2 字段。
type OrderArgs struct {
	TokenID string    `json:"token_id"`
	Price   float64   `json:"price"`
	Size    float64   `json:"size"`
	Side    OrderSide `json:"side"`
	// TickSize 是 SDK 构造/校验签名所需的本地输入；官方 /orders body 没有该字段。
	TickSize   TickSize `json:"tick_size"`
	FeeRateBps *int     `json:"fee_rate_bps,omitempty"`
	Expiration int64    `json:"expiration,omitempty"` // unix 秒；0 = GTC
	PostOnly   bool     `json:"post_only,omitempty"`
	DeferExec  bool     `json:"defer_exec,omitempty"`

	// NegRisk 可选携带该 token 的 NegRisk 状态，用于 V2 type-3（POLY_1271）
	// 签名选 exchange 域。上游若已从 gamma /markets 或 clob 市场数据拿到该状态
	// （二者都带 negRisk 字段），直接传进来即可让批量下单零网络开销地签对域；
	// 留 nil（未知）则由 SDK 现查 GET /neg-risk 兜底。
	NegRisk *bool `json:"neg_risk,omitempty"`
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
	Success            bool                 `json:"success"`
	OrderID            Keccak256            `json:"orderID"`
	Status             string               `json:"status"`
	MakingAmount       string               `json:"makingAmount"`
	TakingAmount       string               `json:"takingAmount"`
	TransactionsHashes []Keccak256          `json:"transactionsHashes,omitempty"`
	TradeIDs           []string             `json:"tradeIDs,omitempty"`
	ErrorMsg           string               `json:"errorMsg"`
	ExpectedOrderID    Keccak256            `json:"-"`
	SettlementState    OrderSettlementState `json:"-"`
	ResolvedTrades     []ClobTrade          `json:"-"`
	Timing             OrderResponseTiming  `json:"-"`
}

// Accepted 表示 CLOB 已明确接收订单。它只判断接单结果，不代表订单已经成交。
func (r OrderPostResponse) Accepted() bool {
	if r.Success {
		return true
	}
	switch strings.ToLower(r.Status) {
	case "live", "matched", "delayed":
		return r.ErrorMsg == ""
	default:
		return false
	}
}

// IsUnknown 表示请求可能已经到达 CLOB，但当前无法确认接单结果。
func (r OrderPostResponse) IsUnknown() bool {
	return strings.EqualFold(r.Status, "unknown")
}

// NeedsSettlement 表示订单已撮合但 SDK 还没有取得交易哈希或明确失败结果。
func (r OrderPostResponse) NeedsSettlement() bool {
	return strings.EqualFold(r.Status, "matched") &&
		len(r.TradeIDs) > 0 &&
		len(r.TransactionsHashes) == 0 &&
		r.SettlementState != OrderSettlementFailed
}

// NeedsFollowUp 表示 Instant 响应仍需通过 AwaitOrderResult(s) 继续对账。
func (r OrderPostResponse) NeedsFollowUp() bool {
	return r.IsUnknown() || r.NeedsSettlement()
}

// DefinitelyNotSubmitted 表示 SDK 能确定该订单没有被 CLOB 接收。
func (r OrderPostResponse) DefinitelyNotSubmitted() bool {
	switch strings.ToLower(r.Status) {
	case "not_submitted", "market_closed", "server_rejected":
		return true
	default:
		return false
	}
}

// SendOrderResponse 是官方文档中的响应名称。保留 OrderPostResponse 作为主类型，
// 避免破坏现有调用方。
type SendOrderResponse = OrderPostResponse

// OrderResponseTiming 记录 SDK 从提交到补全异步成交信息的本地耗时。
// 这些字段不参与 CLOB JSON 编解码，便于调用方汇总 p50/p95/p99。
type OrderResponseTiming struct {
	PostDuration               time.Duration
	ReconciliationWaitDuration time.Duration
	SettlementWaitDuration     time.Duration
	TotalDuration              time.Duration
	ReconciliationPollCount    int
	PollCount                  int
	ReconciliationQueryErrors  int
	QueryErrors                int
	Reconciled                 bool
	ReconciliationTimedOut     bool
	SettlementTimedOut         bool
}

// ClobTradeParams 是认证 GET /data/trades 支持的官方过滤器。
type ClobTradeParams struct {
	ID           string
	MakerAddress EthAddress
	Market       ConditionID
	AssetID      TokenID
	Before       int64
	After        int64
}

// ClobTradeStatus 表示 CLOB 成交执行状态。
type ClobTradeStatus string

// UnmarshalJSON 同时兼容官方文档中的 TRADE_STATUS_CONFIRMED 形式和生产
// SDK 当前使用的 CONFIRMED 形式，并统一为短枚举值。
func (s *ClobTradeStatus) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	value = strings.TrimPrefix(strings.ToUpper(value), "TRADE_STATUS_")
	*s = ClobTradeStatus(value)
	return nil
}

const (
	ClobTradeStatusMatched   ClobTradeStatus = "MATCHED"
	ClobTradeStatusMined     ClobTradeStatus = "MINED"
	ClobTradeStatusConfirmed ClobTradeStatus = "CONFIRMED"
	ClobTradeStatusRetrying  ClobTradeStatus = "RETRYING"
	ClobTradeStatusFailed    ClobTradeStatus = "FAILED"
)

// ClobMakerOrder 是一笔成交中参与撮合的 maker 订单。
type ClobMakerOrder struct {
	OrderID       Keccak256  `json:"order_id"`
	Owner         string     `json:"owner"`
	MakerAddress  EthAddress `json:"maker_address"`
	MatchedAmount string     `json:"matched_amount"`
	Price         string     `json:"price"`
	FeeRateBPS    string     `json:"fee_rate_bps"`
	AssetID       TokenID    `json:"asset_id"`
	Outcome       string     `json:"outcome"`
	Side          OrderSide  `json:"side"`
}

// ClobTrade 表示认证 CLOB GET /data/trades 返回的一笔成交。金额与价格保留
// 官方十进制定点字符串，避免 float64 精度损失。
type ClobTrade struct {
	ID              string           `json:"id"`
	TakerOrderID    Keccak256        `json:"taker_order_id"`
	Market          ConditionID      `json:"market"`
	AssetID         TokenID          `json:"asset_id"`
	Side            OrderSide        `json:"side"`
	Size            string           `json:"size"`
	FeeRateBPS      string           `json:"fee_rate_bps"`
	Price           string           `json:"price"`
	Status          ClobTradeStatus  `json:"status"`
	MatchTime       string           `json:"match_time"`
	LastUpdate      string           `json:"last_update"`
	Outcome         string           `json:"outcome"`
	BucketIndex     int              `json:"bucket_index"`
	Owner           string           `json:"owner"`
	MakerAddress    EthAddress       `json:"maker_address"`
	TransactionHash Keccak256        `json:"transaction_hash"`
	TraderSide      string           `json:"trader_side"`
	ErrorMsg        string           `json:"err_msg"`
	MakerOrders     []ClobMakerOrder `json:"maker_orders"`
}

// OrderSettlementState 是 SDK 观察到的订单成交结算阶段。
type OrderSettlementState string

const (
	OrderSettlementMatched   OrderSettlementState = "matched"
	OrderSettlementMined     OrderSettlementState = "mined"
	OrderSettlementConfirmed OrderSettlementState = "confirmed"
	OrderSettlementFailed    OrderSettlementState = "failed"
	OrderSettlementTimeout   OrderSettlementState = "timeout"
)

// OrderFillSettlement 汇总一笔订单关联的全部成交及当前结算阶段。
type OrderFillSettlement struct {
	State              OrderSettlementState
	Trades             []ClobTrade
	TransactionsHashes []Keccak256
	Timing             OrderResponseTiming
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

// Polymarket 当前支持的价格步长。
// Go 没有封闭枚举；业务代码应优先使用这些常量，避免散落字符串魔数。
const (
	TickSize0_1    TickSize = "0.1"
	TickSize0_01   TickSize = "0.01"
	TickSize0_005  TickSize = "0.005"
	TickSize0_0025 TickSize = "0.0025"
	TickSize0_001  TickSize = "0.001"
	TickSize0_0001 TickSize = "0.0001"
)

// TickSizeFromFloat 把业务层的浮点配置转换为 OrderArgs 使用的十进制 tick size。
// 合法性仍由下单前校验负责；本函数只避免各业务重复编写 FormatFloat。
func TickSizeFromFloat(value float64) TickSize {
	return TickSize(strconv.FormatFloat(value, 'f', -1, 64))
}

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

// BalanceAllowance 表示余额授权信息。V2 服务端把 balance/allowance 序列化为字符串
// （最小单位的 uint256，例如 "43468934"），所以这里用 string 承接再由调用方解析。
type BalanceAllowance struct {
	Allowance string `json:"allowance"`
	Balance   string `json:"balance"`
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
