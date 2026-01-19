package internal

import (
	"time"

	"github.com/polymarket/go-polymarket-sdk/types"
)

// ============================================================================
// 基础常量
// ============================================================================

const (
	// Address constants
	AddressZero = "0x0000000000000000000000000000000000000000"
	HashZero    = "0x0000000000000000000000000000000000000000000000000000000000000000"

	// Pagination
	EndCursor = "LTE="
)

// ============================================================================
// API 域名和基础URL
// ============================================================================

const (
	// Polygon RPC 节点
	PolygonRPCMainnet = "https://rpc-mainnet.matic.quiknode.pro"
	PolygonRPCAmoy    = "https://rpc-amoy.polygon.technology"

	// Polymarket API 服务
	ClobAPIDomain  = "https://clob.polymarket.com"
	GammaAPIDomain = "https://gamma-api.polymarket.com"
	DataAPIDomain  = "https://data-api.polymarket.com"
	RelayerDomain  = "https://relayer-v2.polymarket.com"
)

// ============================================================================
// API 端点路径
// ============================================================================

// System endpoints
const (
	Time = "/time"
)

// API Keys endpoints
const (
	CreateAPIKey         = "/auth/api-key"
	GetAPIKeys           = "/auth/api-keys"
	DeleteAPIKey         = "/auth/api-key"
	DeriveAPIKey         = "/auth/derive-api-key"
	CreateReadonlyAPIKey = "/auth/readonly-api-key"
	GetReadonlyAPIKeys   = "/auth/readonly-api-keys"
	DeleteReadonlyAPIKey = "/auth/readonly-api-key"
)

// Orders endpoints
const (
	PostOrder          = "/order"
	PostOrders         = "/orders"
	Cancel             = "/order"
	CancelOrders       = "/orders"
	CancelAll          = "/cancel-all"
	CancelMarketOrders = "/cancel-market-orders"
	Orders             = "/data/orders"
)

// Order Books endpoints
const (
	GetOrderBook  = "/book"
	GetOrderBooks = "/books"
)

// Pricing endpoints
const (
	MidPoint            = "/midpoint"
	MidPoints           = "/midpoints"
	Price               = "/price"
	GetPrices           = "/prices"
	GetSpread           = "/spread"
	GetSpreads          = "/spreads"
	GetLastTradePrice   = "/last-trade-price"
	GetLastTradesPrices = "/last-trades-prices"
)

// Trades endpoints
const (
	Trades = "/data/trades"
)

// Markets endpoints
const (
	GetSamplingSimplifiedMarkets = "/sampling-simplified-markets"
	GetSamplingMarkets           = "/sampling-markets"
	GetSimplifiedMarkets         = "/simplified-markets"
	GetMarkets                   = "/markets"
	GetMarket                    = "/markets/"
	GetMarketTradesEvents        = "/live-activity/events/"
)

// Market Data endpoints
const (
	GetTickSize = "/tick-size"
	GetNegRisk  = "/neg-risk"
	GetFeeRate  = "/fee-rate"
)

// Rewards endpoints
const (
	IsOrderScoring   = "/order-scoring"
	AreOrdersScoring = "/orders-scoring"
)

// Balance endpoints
const (
	GetBalanceAllowance    = "/balance-allowance"
	UpdateBalanceAllowance = "/balance-allowance/update"
)

// Notifications endpoints
const (
	GetNotifications  = "/notifications"
	DropNotifications = "/notifications"
)

// ============================================================================
// 时间相关常量
// ============================================================================

const (
	// HTTP 客户端超时时间
	HTTPClientTimeout     = 30 * time.Second
	HTTPClientLongTimeout = 60 * time.Second

	// WebSocket 相关
	WebSocketDialTimeout      = 60 * time.Second
	WebSocketHandshakeTimeout = 60 * time.Second
	WebSocketKeepAlive        = 30 * time.Second

	// Relay 相关
	RelayNonceMaxRetries = 3                // Relay nonce 最大重试次数
	RelayNonceTimeout    = 30 * time.Second // Relay nonce 请求超时

	// 交易执行延迟
	TransactionDelay = 2 * time.Second // 批量交易间的延迟

	// 交易等待超时
	TransactionWaitTimeout = 5 * time.Minute
)

// ============================================================================
// 限制和阈值常量
// ============================================================================

const (
	// 查询限制
	DefaultPositionLimit = 100 // 默认位置查询限制

	// Gas 估算相关
	DefaultGasEstimate    = 10_000_000 // 默认 gas 估算值
	GasEstimateMultiplier = 130        // Gas 估算倍数（1.3x，以百分比表示）
	GasEstimateExtra      = 100_000    // Gas 估算额外值（100k）

	// 数量精度
	QuantityPrecision = 100000.0 // 数量精度（用于四舍五入）

	// 预览长度
	APIResponsePreviewLen = 1000 // API 响应预览长度
)

// ============================================================================
// 策略相关常量
// ============================================================================

const (
	// 订单相关
	// MinOrderSize 最小订单大小（单位：shares）
	// Polymarket API 要求最小订单大小，低于此值的订单会被拒绝
	MinOrderSize float64 = 0.1

	// DefaultTickSize 默认价格 tick 大小
	// 用于价格量化，确保价格符合交易所要求
	DefaultTickSize = 0.001

	// SizeQuantizationMultiplier 数量量化倍数
	// 用于 QuantizeSize 函数，将数量量化为 5 位小数
	// 计算公式：math.Floor(size * SizeQuantizationMultiplier) / SizeQuantizationMultiplier
	SizeQuantizationMultiplier = 100000.0

	// MinQuantizedSize 最小量化后的数量
	// 量化后的数量不能小于此值（5 位小数精度）
	MinQuantizedSize = 0.00001

	// 重试相关
	// MaxRetries 最大重试次数
	// 用于临时性错误（如 HTTP 502、500、timeout）的重试
	MaxRetries = 3

	// RetryBackoffBase 重试退避基础时间（单位：秒）
	// 重试延迟 = RetryBackoffBase * 重试次数
	// 例如：第1次重试延迟 5 秒，第2次延迟 10 秒，第3次延迟 15 秒
	RetryBackoffBase = 5 * time.Second
)

// ============================================================================
// 订单状态相关
// ============================================================================

// OpenOrderStatuses 开放状态的订单状态集合
// 用于判断订单是否处于可取消状态
// 这些状态的订单可以被取消：LIVE、OPEN、PARTIALLY_FILLED、ACCEPTED
// 使用方式：internal.OpenOrderStatuses[order.Status]
var OpenOrderStatuses = map[string]bool{
	"LIVE":             true,
	"OPEN":             true,
	"PARTIALLY_FILLED": true,
	"ACCEPTED":         true,
}

// ============================================================================
// Chain IDs
// ============================================================================

const (
	// Polygon mainnet
	Polygon types.ChainID = 137
	// Amoy testnet
	Amoy types.ChainID = 80002
)
