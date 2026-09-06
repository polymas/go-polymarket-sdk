package clob

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/polymas/go-polymarket-sdk/internal"
	http "github.com/polymas/go-polymarket-sdk/internal/transport"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

// negRiskPrefetchConcurrency 限制并发预取 GET /neg-risk 的 goroutine 数，
// 既压缩墙钟时间又不至于把 CLOB 网关打挂（批量上限 15，8 路足够）。
const negRiskPrefetchConcurrency = 8

// OrderClient 订单相关操作的轻量接口
type OrderClient interface {
	GetOrder(orderID types.Keccak256) (*types.OpenOrder, error)
	GetOrderContext(ctx context.Context, orderID types.Keccak256) (*types.OpenOrder, error)
	GetOrders(orderID *types.Keccak256, conditionID *types.Keccak256, tokenID *string) ([]types.OpenOrder, error)
	GetOrdersContext(ctx context.Context, orderID *types.Keccak256, conditionID *types.Keccak256, tokenID *string) ([]types.OpenOrder, error)
	GetTrades(params *types.ClobTradeParams) ([]types.ClobTrade, error)
	GetTradesContext(ctx context.Context, params *types.ClobTradeParams) ([]types.ClobTrade, error)
	WaitForOrderFillSettlement(ctx context.Context, response types.OrderPostResponse) (*types.OrderFillSettlement, error)
	AwaitOrderResult(ctx context.Context, response types.OrderPostResponse) (*types.OrderPostResponse, error)
	AwaitOrderResults(ctx context.Context, responses []types.OrderPostResponse) ([]types.OrderPostResponse, error)
	// RecordTradeUpdate 把 user WebSocket 频道推送的 trade 事件喂给 SDK。喂入后
	// 结算等待（AndWait / AwaitOrderResults / WaitForOrderFillSettlement）优先
	// 消费推送，HTTP 轮询退化为 2 秒一次的兜底。用 websocket.UserTradeEvent 的
	// ToClobTrade 转换后传入即可；从不调用则保持纯轮询行为。
	RecordTradeUpdate(trade types.ClobTrade)
	CreateAndPostOrders(orderArgsList []types.OrderArgs, orderTypes []types.OrderType) ([]types.OrderPostResponse, error)
	CreateAndPostOrdersContext(ctx context.Context, orderArgsList []types.OrderArgs, orderTypes []types.OrderType) ([]types.OrderPostResponse, error)
	CreateAndPostOrdersInstant(orderArgsList []types.OrderArgs, orderTypes []types.OrderType) ([]types.OrderPostResponse, error)
	CreateAndPostOrdersInstantContext(ctx context.Context, orderArgsList []types.OrderArgs, orderTypes []types.OrderType) ([]types.OrderPostResponse, error)
	CreateAndPostOrdersAndWait(ctx context.Context, orderArgsList []types.OrderArgs, orderTypes []types.OrderType) ([]types.OrderPostResponse, error)
	CancelOrders(orderIDs []types.Keccak256) (*types.OrderCancelResponse, error)
	CancelOrdersContext(ctx context.Context, orderIDs []types.Keccak256) (*types.OrderCancelResponse, error)
	CancelAll() (*types.OrderCancelResponse, error)
	CancelAllContext(ctx context.Context) (*types.OrderCancelResponse, error)
	PostOrder(orderArgs types.OrderArgs, orderType types.OrderType) (*types.OrderPostResponse, error)
	PostOrderContext(ctx context.Context, orderArgs types.OrderArgs, orderType types.OrderType) (*types.OrderPostResponse, error)
	PostOrderInstant(orderArgs types.OrderArgs, orderType types.OrderType) (*types.OrderPostResponse, error)
	PostOrderInstantContext(ctx context.Context, orderArgs types.OrderArgs, orderType types.OrderType) (*types.OrderPostResponse, error)
	PostOrderAndWait(ctx context.Context, orderArgs types.OrderArgs, orderType types.OrderType) (*types.OrderPostResponse, error)
	CreateAndPostMarketOrder(orderArgs types.MarketOrderArgs) (*types.OrderPostResponse, error)
	CreateAndPostMarketOrderContext(ctx context.Context, orderArgs types.MarketOrderArgs) (*types.OrderPostResponse, error)
	CreateAndPostMarketOrderInstant(orderArgs types.MarketOrderArgs) (*types.OrderPostResponse, error)
	CreateAndPostMarketOrderInstantContext(ctx context.Context, orderArgs types.MarketOrderArgs) (*types.OrderPostResponse, error)
	CreateAndPostMarketOrderAndWait(ctx context.Context, orderArgs types.MarketOrderArgs) (*types.OrderPostResponse, error)
	CancelOrder(orderID types.Keccak256) (*types.OrderCancelResponse, error)
	CancelOrderContext(ctx context.Context, orderID types.Keccak256) (*types.OrderCancelResponse, error)
	CancelMarketOrders(conditionID types.Keccak256) (*types.OrderCancelResponse, error)
	CancelMarketOrdersContext(ctx context.Context, conditionID types.Keccak256) (*types.OrderCancelResponse, error)
	CancelMarketOrdersByFilter(params types.CancelMarketOrdersParams) (*types.OrderCancelResponse, error)
	CancelMarketOrdersByFilterContext(ctx context.Context, params types.CancelMarketOrdersParams) (*types.OrderCancelResponse, error)
}

// MarketDataClient 市场数据相关操作的轻量接口
type MarketDataClient interface {
	GetOrderBook(tokenID string) (*types.OrderBookSummary, error)
	GetOrderBookContext(ctx context.Context, tokenID string) (*types.OrderBookSummary, error)
	GetMultipleOrderBooks(requests []types.BookParams) ([]types.OrderBookSummary, error)
	GetMultipleOrderBooksContext(ctx context.Context, requests []types.BookParams) ([]types.OrderBookSummary, error)
	GetMidpoint(tokenID string) (*types.Midpoint, error)
	GetMidpointContext(ctx context.Context, tokenID string) (*types.Midpoint, error)
	GetMidpoints(tokenIDs []string) ([]types.Midpoint, error)
	GetMidpointsContext(ctx context.Context, tokenIDs []string) ([]types.Midpoint, error)
	GetPrice(tokenID string, side types.OrderSide) (*types.Price, error)
	GetPriceContext(ctx context.Context, tokenID string, side types.OrderSide) (*types.Price, error)
	GetPrices(requests []types.BookParams) ([]types.Price, error)
	GetPricesContext(ctx context.Context, requests []types.BookParams) ([]types.Price, error)
	GetPricesHistory(market string, opts ...PricesHistoryOption) (*types.PricesHistoryResponse, error)
	GetPricesHistoryContext(ctx context.Context, market string, opts ...PricesHistoryOption) (*types.PricesHistoryResponse, error)
	GetSpread(tokenID string) (*types.Spread, error)
	GetSpreadContext(ctx context.Context, tokenID string) (*types.Spread, error)
	GetSpreads(tokenIDs []string) ([]types.Spread, error)
	GetSpreadsContext(ctx context.Context, tokenIDs []string) ([]types.Spread, error)
	GetLastTradePrice(tokenID string) (*types.LastTradePrice, error)
	GetLastTradePriceContext(ctx context.Context, tokenID string) (*types.LastTradePrice, error)
	GetLastTradesPrices(tokenIDs []string) (map[types.TokenID]types.LastTradePrice, error)
	GetLastTradesPricesContext(ctx context.Context, tokenIDs []string) (map[types.TokenID]types.LastTradePrice, error)
	// CalculateMarketPrice 按当前订单簿估算市价单的最差成交价。
	// BUY 的 amount 是 pUSD 金额，SELL 的 amount 是 shares。
	CalculateMarketPrice(tokenID string, side types.OrderSide, amount float64, orderType types.OrderType) (float64, error)
	CalculateMarketPriceContext(ctx context.Context, tokenID string, side types.OrderSide, amount float64, orderType types.OrderType) (float64, error)
	// GetTickSize 显式查询 token 当前的最小价格步长；SDK 不隐藏缓存。
	// 下单方法不会隐式调用；业务层可选择默认值或提前调用本方法。
	GetTickSize(tokenID string) (types.TickSize, error)
	GetTickSizeContext(ctx context.Context, tokenID string) (types.TickSize, error)
	GetNegRisk(tokenID string) (bool, error)
	GetNegRiskContext(ctx context.Context, tokenID string) (bool, error)
	PrimeNegRisk(tokenID string, value bool) error
	InvalidateNegRisk(tokenID string) error
	GetTime() (time.Time, error)
	GetTimeContext(ctx context.Context) (time.Time, error)
}

// AccountClient 账户相关操作的轻量接口
type AccountClient interface {
	// GetCollateralBalance returns the current V2 trading collateral (pUSD).
	GetCollateralBalance() (float64, error)
	// Deprecated: GetUSDCBalance returns legacy USDC.e, not V2 collateral.
	// Use GetCollateralBalance for trading funds.
	GetUSDCBalance() (float64, error)
	GetBalanceAllowance() (*types.BalanceAllowance, error)
	GetBalanceAllowanceContext(ctx context.Context) (*types.BalanceAllowance, error)
	UpdateBalanceAllowance(amount float64) (*types.BalanceAllowance, error)
	UpdateBalanceAllowanceContext(ctx context.Context, amount float64) (*types.BalanceAllowance, error)
	GetNotifications(limit int, offset int) ([]types.Notification, error)
	GetNotificationsContext(ctx context.Context, limit int, offset int) ([]types.Notification, error)
	DropNotifications(notificationIDs []string) error
	DropNotificationsContext(ctx context.Context, notificationIDs []string) error
}

// APIKeyClient API Keys 管理相关操作的轻量接口
type APIKeyClient interface {
	GetAPIKeys() ([]types.APIKey, error)
	GetAPIKeysContext(ctx context.Context) ([]types.APIKey, error)
	DeleteAPIKey(keyID string) error
	DeleteAPIKeyContext(ctx context.Context, keyID string) error
	CreateReadonlyAPIKey() (*types.APIKey, error)
	CreateReadonlyAPIKeyContext(ctx context.Context) (*types.APIKey, error)
	GetReadonlyAPIKeys() ([]types.APIKey, error)
	GetReadonlyAPIKeysContext(ctx context.Context) ([]types.APIKey, error)
	DeleteReadonlyAPIKey(keyID string) error
	DeleteReadonlyAPIKeyContext(ctx context.Context, keyID string) error
}

// RewardClient 奖励相关操作的轻量接口
type RewardClient interface {
	IsOrderScoring(orderID types.Keccak256) (bool, error)
	IsOrderScoringContext(ctx context.Context, orderID types.Keccak256) (bool, error)
	AreOrdersScoring(orderIDs []types.Keccak256) (map[types.Keccak256]bool, error)
	AreOrdersScoringContext(ctx context.Context, orderIDs []types.Keccak256) (map[types.Keccak256]bool, error)
}

// ReadonlyClient 只读客户端接口，不需要私钥和API凭证
// 包含所有公开的市场数据和奖励查询接口
type ReadonlyClient interface {
	MarketDataClient
	RewardClient
}

// Client 定义CLOB客户端的完整接口，通过组合各个功能接口实现
// 需要私钥和API凭证，包含所有功能
type Client interface {
	ReadonlyClient // 继承只读接口
	OrderClient
	AccountClient
	APIKeyClient

	// GetAPICreds 返回初始化时派生的 API 凭证，用于需要 Level2 鉴权的外部调用（如 GaslessClient）
	GetAPICreds() *types.ApiCreds
}

// baseClient 基础客户端结构，包含所有共享的字段和方法
type baseClient struct {
	address       types.EthAddress // Base address
	proxyAddress  types.EthAddress // Proxy address (for proxy wallets), cached
	baseURL       string           // API 基础 URL
	signatureType types.SignatureType
	deriveCreds   *types.ApiCreds
	negRisk       *negRiskCache
	web3Client    web3.Client // 保存 Web3Client 引用（可能为nil，用于只读客户端）

	tradeFeedOnce sync.Once
	tradeFeed     *tradeFeed // 业务层喂入的 user channel trade 事件，见 RecordTradeUpdate
}

// trades 返回懒初始化的 trade 事件缓存，便于测试里直接构造 baseClient。
func (c *baseClient) trades() *tradeFeed {
	c.tradeFeedOnce.Do(func() {
		if c.tradeFeed == nil {
			c.tradeFeed = newTradeFeed()
		}
	})
	return c.tradeFeed
}

// ClientOption 客户端构造选项
type ClientOption func(*baseClient)

// WithV2 显式选择当前 V2 订单协议。
//
// V2 已是 NewClient 的默认值；保留该 option 仅用于源码兼容以及让调用方可以
// 显式表达意图。host 始终使用 ClobAPIDomain。
//
// Deprecated: V2 is the default; omit this option.
func WithV2() ClientOption {
	return func(*baseClient) {}
}

// readonlyBaseClient 只读客户端的基础结构，不包含认证相关字段
type readonlyBaseClient struct {
	baseURL string
	negRisk *negRiskCache
}

// orderClientImpl 订单功能模块实现
type orderClientImpl struct {
	*baseClient
}

// marketDataClientImpl 市场数据功能模块实现
type marketDataClientImpl struct {
	*baseClient
}

// accountClientImpl 账户功能模块实现
type accountClientImpl struct {
	*baseClient
}

// apiKeyClientImpl API Keys 管理功能模块实现
type apiKeyClientImpl struct {
	*baseClient
}

// rewardClientImpl 奖励功能模块实现
type rewardClientImpl struct {
	*baseClient
}

// readonlyMarketDataClientImpl 只读市场数据功能模块实现
type readonlyMarketDataClientImpl struct {
	*readonlyBaseClient
}

// readonlyRewardClientImpl 只读奖励功能模块实现
type readonlyRewardClientImpl struct {
	*readonlyBaseClient
}

// readonlyClobClient 只读CLOB客户端实现
// 不需要私钥和API凭证，只能使用公开接口
type readonlyClobClient struct {
	*readonlyBaseClient
	*readonlyMarketDataClientImpl
	*readonlyRewardClientImpl
}

// polymarketClobClient 处理CLOB（中央限价订单簿）操作
// 通过组合各个功能模块实现完整的 Client 接口
// 不允许直接导出，只能通过 NewClient 创建
type polymarketClobClient struct {
	*baseClient
	*orderClientImpl
	*marketDataClientImpl
	*accountClientImpl
	*apiKeyClientImpl
	*rewardClientImpl
}

// NewReadonlyClient 创建只读CLOB客户端
// 不需要私钥和API凭证，只能使用公开的市场数据和奖励查询接口
// 返回 ReadonlyClient 接口
func NewReadonlyClient() ReadonlyClient {
	// 创建只读基础客户端
	readonlyBase := &readonlyBaseClient{
		baseURL: internal.ClobAPIDomain,
		negRisk: newNegRiskCache(),
	}

	// 创建功能模块
	marketDataClient := &readonlyMarketDataClientImpl{readonlyBaseClient: readonlyBase}
	rewardClient := &readonlyRewardClientImpl{readonlyBaseClient: readonlyBase}

	// 组合只读功能模块
	return &readonlyClobClient{
		readonlyBaseClient:           readonlyBase,
		readonlyMarketDataClientImpl: marketDataClient,
		readonlyRewardClientImpl:     rewardClient,
	}
}

// NewClient 创建新的完整CLOB客户端
// 需要私钥和API凭证，可以使用所有功能接口
// 在初始化时自动调用 createOrDeriveAPICreds 获取 API 凭证
// 返回 Client 接口，不允许直接访问实现类型
//
// 只使用当前 V2 订单协议。生产 CLOB 不再兼容 V1；WithV2() 仅为源码兼容
// 保留，已经不再需要。
func NewClient(web3Client web3.Client, opts ...ClientOption) (Client, error) {
	return NewClientContext(context.Background(), web3Client, opts...)
}

// NewClientContext initializes a CLOB client while allowing API credential
// creation/derivation to be canceled by the caller.
func NewClientContext(ctx context.Context, web3Client web3.Client, opts ...ClientOption) (Client, error) {
	// 从 web3.Client 获取所需信息
	signatureType := web3Client.GetSignatureType()
	address := web3Client.GetBaseAddress()

	// 创建基础客户端
	base := &baseClient{
		address:       address,
		proxyAddress:  "", // Will be set in initialization
		baseURL:       internal.ClobAPIDomain,
		signatureType: signatureType,
		negRisk:       newNegRiskCache(),
		web3Client:    web3Client,
	}

	// 应用可选配置；后传入的 option 覆盖前面的设置。
	for _, opt := range opts {
		opt(base)
	}

	// 自动创建或派生 API 凭证
	derivedCreds, err := base.CreateOrDeriveAPICredsContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create/derive API creds: %w", err)
	}
	base.deriveCreds = derivedCreds

	// 初始化时获取 proxy address
	proxyAddr, err := web3Client.GetPolyProxyAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to get proxy address: %w", err)
	}
	base.proxyAddress = proxyAddr

	internal.LogInfo(
		"CLOB client initialized: protocol=V2 signature_type=%d signer=%s maker=%s exchange_domains=%s,%s",
		base.signatureType,
		base.address,
		base.proxyAddress,
		internal.PolygonExchangeV2,
		internal.PolygonNegRiskExchangeV2,
	)

	// 创建功能模块
	orderClient := &orderClientImpl{baseClient: base}
	marketDataClient := &marketDataClientImpl{baseClient: base}
	accountClient := &accountClientImpl{baseClient: base}
	apiKeyClient := &apiKeyClientImpl{baseClient: base}
	rewardClient := &rewardClientImpl{baseClient: base}

	// 组合所有功能模块
	clobClient := &polymarketClobClient{
		baseClient:           base,
		orderClientImpl:      orderClient,
		marketDataClientImpl: marketDataClient,
		accountClientImpl:    accountClient,
		apiKeyClientImpl:     apiKeyClient,
		rewardClientImpl:     rewardClient,
	}

	return clobClient, nil
}

// GetAPICreds 返回初始化时派生/创建的 API 凭证（不复制，调用方不得修改）
func (c *baseClient) GetAPICreds() *types.ApiCreds {
	return c.deriveCreds
}

// negRiskForToken 查询单个 token 是否属于 NegRisk 市场，带 c.negRisk 缓存。
//
// V2 type-3（CWIA / POLY_1271）签名的 verifyingContract 域取决于此：
// 非 NegRisk → ExchangeV2，NegRisk → NegRiskExchangeV2。批量下单必须逐 token
// 取真实值，否则 NegRisk 市场的单子会按错误的 exchange 域签名，服务端报
// "invalid POLY_1271 signature: signature does not match order hash"。
//
// 与 marketDataClientImpl.GetNegRisk 共用 baseClient.negRisk 缓存；查不到时
// 退回 false，由调用方的 negRisk 重试兜底。
func (c *baseClient) negRiskForToken(tokenID string) (bool, error) {
	return c.negRiskForTokenContext(context.Background(), tokenID)
}

func (c *baseClient) negRiskForTokenContext(ctx context.Context, tokenID string) (bool, error) {
	canonical, err := canonicalNegRiskTokenID(tokenID)
	if err != nil {
		return false, err
	}
	return getNegRiskCachedContext(ctx, c.baseURL, c.negRisk, canonical)
}

// negRiskFetchList 计算一批订单里"NegRisk 状态未知、需要现查"的去重 tokenID
// 列表：跳过调用方已传入 NegRisk 的、以及缓存已命中的，按首次出现顺序去重。
// 抽成纯函数便于离线单测（见 prefetch_negrisk_test.go）。
func (c *baseClient) negRiskFetchList(orderArgsList []types.OrderArgs) []string {
	need := make([]string, 0, len(orderArgsList))
	seen := make(map[string]struct{}, len(orderArgsList))
	for i := range orderArgsList {
		if orderArgsList[i].NegRisk != nil {
			continue // 调用方已传入真实值，签名循环直接用
		}
		id, err := canonicalNegRiskTokenID(orderArgsList[i].TokenID)
		if err != nil {
			continue
		}
		if _, ok := c.negRisk.get(id); ok {
			continue // 缓存已命中
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		need = append(need, id)
	}
	return need
}

// prefetchNegRisk 并发预取一批订单中"NegRisk 状态未知"的 token，填充
// baseClient.negRisk 缓存，避免签名循环里逐个串行 GET /neg-risk。
//
// 只处理 orderArgs.NegRisk == nil（调用方未传入）且缓存未命中的去重 token；
// 已传入或已缓存的跳过。≤1 个待查时不起 goroutine，留给签名循环里的
// negRiskForToken 串行兜底即可。
//
// 并发安全：缓存自身加锁，同一 token 的并发冷启动查询由 singleflight 合并。
func (c *baseClient) prefetchNegRisk(orderArgsList []types.OrderArgs) {
	c.prefetchNegRiskContext(context.Background(), orderArgsList)
}

func (c *baseClient) prefetchNegRiskContext(ctx context.Context, orderArgsList []types.OrderArgs) {
	need := c.negRiskFetchList(orderArgsList)
	if len(need) <= 1 {
		return // 0 或 1 个，串行查即可，不值得起 goroutine
	}

	sem := make(chan struct{}, negRiskPrefetchConcurrency)
	var wg sync.WaitGroup
	for _, id := range need {
		wg.Add(1)
		sem <- struct{}{}
		go func(id string) {
			defer wg.Done()
			defer func() { <-sem }()
			_, _ = getNegRiskCachedContext(ctx, c.baseURL, c.negRisk, id)
		}(id)
	}
	wg.Wait()
}

// CreateOrDeriveAPICreds creates or derives API credentials
func (c *baseClient) CreateOrDeriveAPICreds() (*types.ApiCreds, error) {
	return c.CreateOrDeriveAPICredsContext(context.Background())
}

func (c *baseClient) CreateOrDeriveAPICredsContext(ctx context.Context) (*types.ApiCreds, error) {
	// Try to create first
	headers, err := internal.CreateLevel1Headers(c.web3Client.GetSigner(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create level 1 headers: %w", err)
	}

	creds, err := http.PostContext[types.ApiCreds](ctx, c.baseURL, internal.CreateAPIKey, nil, http.WithHeaders(headers), http.WithService("clob"), http.WithAmbiguousOnTimeout("create API credentials"))
	if err != nil {
		// If creation fails, try to derive (need to recreate headers for GET request)
		headers, err = internal.CreateLevel1Headers(c.web3Client.GetSigner(), nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create level 1 headers for derive: %w", err)
		}
		creds, err = http.GetContext[types.ApiCreds](ctx, c.baseURL, internal.DeriveAPIKey, nil, http.WithHeaders(headers), http.WithService("clob"))
		if err != nil {
			return nil, fmt.Errorf("failed to create or derive API creds: %w", err)
		}
	}

	return creds, nil
}
