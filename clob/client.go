package clob

import (
	"fmt"
	"math/big"
	"time"

	"github.com/polymarket/go-order-utils/pkg/builder"
	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

// OrderClient 订单相关操作的轻量接口
type OrderClient interface {
	GetOrders(orderID *types.Keccak256, conditionID *types.Keccak256, tokenID *string) ([]types.OpenOrder, error)
	CreateAndPostOrders(orderArgsList []types.OrderArgs, orderTypes []types.OrderType) ([]types.OrderPostResponse, error)
	CancelOrders(orderIDs []types.Keccak256) (*types.OrderCancelResponse, error)
	CancelAll() (*types.OrderCancelResponse, error)
	PostOrder(orderArgs types.OrderArgs, orderType types.OrderType) (*types.OrderPostResponse, error)
	CancelOrder(orderID types.Keccak256) (*types.OrderCancelResponse, error)
	CancelMarketOrders(conditionID types.Keccak256) (*types.OrderCancelResponse, error)
}

// MarketDataClient 市场数据相关操作的轻量接口
type MarketDataClient interface {
	GetOrderBook(tokenID string) (*types.OrderBookSummary, error)
	GetMultipleOrderBooks(requests []types.BookParams) ([]types.OrderBookSummaryResponse, error)
	GetMidpoint(tokenID string) (*types.Midpoint, error)
	GetMidpoints(tokenIDs []string) ([]types.Midpoint, error)
	GetPrice(tokenID string, side types.OrderSide) (*types.Price, error)
	GetPrices(requests []types.BookParams) ([]types.Price, error)
	GetSpread(tokenID string) (*types.Spread, error)
	GetSpreads(tokenIDs []string) ([]types.Spread, error)
	GetLastTradePrice(tokenID string) (*types.LastTradePrice, error)
	GetLastTradesPrices(tokenIDs []string) ([]types.LastTradePrice, error)
	GetFeeRate(tokenID string) (int, error)
	GetTime() (time.Time, error)
}

// AccountClient 账户相关操作的轻量接口
type AccountClient interface {
	GetUSDCBalance() (float64, error)
	GetBalanceAllowance() (*types.BalanceAllowance, error)
	UpdateBalanceAllowance(amount float64) (*types.BalanceAllowance, error)
	GetNotifications(limit int, offset int) ([]types.Notification, error)
	DropNotifications(notificationIDs []string) error
}

// APIKeyClient API Keys 管理相关操作的轻量接口
type APIKeyClient interface {
	GetAPIKeys() ([]types.APIKey, error)
	DeleteAPIKey(keyID string) error
	CreateReadonlyAPIKey() (*types.APIKey, error)
	GetReadonlyAPIKeys() ([]types.APIKey, error)
	DeleteReadonlyAPIKey(keyID string) error
}

// RewardClient 奖励相关操作的轻量接口
type RewardClient interface {
	IsOrderScoring(orderID types.Keccak256) (bool, error)
	AreOrdersScoring(orderIDs []types.Keccak256) (map[types.Keccak256]bool, error)
}

// Client 定义CLOB客户端的接口，通过组合各个功能接口实现
type Client interface {
	OrderClient
	MarketDataClient
	AccountClient
	APIKeyClient
	RewardClient
}

// baseClient 基础客户端结构，包含所有共享的字段和方法
type baseClient struct {
	address       types.EthAddress // Base address
	proxyAddress  types.EthAddress // Proxy address (for proxy wallets), cached
	baseURL       string           // API 基础 URL
	signatureType types.SignatureType
	deriveCreds   *types.ApiCreds
	tickSizes     map[string]types.TickSize
	negRisk       map[string]bool
	feeRates      map[string]int
	orderBuilder  *builder.ExchangeOrderBuilderImpl
	web3Client    web3.Client // 保存 Web3Client 引用
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

// NewClient 创建新的CLOB客户端
// 在初始化时自动调用 createOrDeriveAPICreds 获取 API 凭证
// 返回 ClobClient 接口，不允许直接访问实现类型
func NewClient(web3Client web3.Client) (Client, error) {
	// 从 web3.Client 获取所需信息
	signatureType := web3Client.GetSignatureType()
	address := web3Client.GetBaseAddress()

	// Create order builder
	chainIDBig := big.NewInt(int64(web3Client.GetChainID()))
	orderBuilder := builder.NewExchangeOrderBuilderImpl(chainIDBig, func() int64 {
		return time.Now().UnixNano()
	})

	// 创建基础客户端
	base := &baseClient{
		address:       address,
		proxyAddress:  "", // Will be set in initialization
		baseURL:       internal.ClobAPIDomain,
		signatureType: signatureType,
		tickSizes:     make(map[string]types.TickSize),
		negRisk:       make(map[string]bool),
		feeRates:      make(map[string]int),
		orderBuilder:  orderBuilder,
		web3Client:    web3Client,
	}

	// 自动创建或派生 API 凭证
	derivedCreds, err := base.CreateOrDeriveAPICreds()
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

// CreateOrDeriveAPICreds creates or derives API credentials
func (c *baseClient) CreateOrDeriveAPICreds() (*types.ApiCreds, error) {
	// Try to create first
	headers, err := internal.CreateLevel1Headers(c.web3Client.GetSigner(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create level 1 headers: %w", err)
	}

	creds, err := http.Post[types.ApiCreds](c.baseURL, internal.CreateAPIKey, nil, http.WithHeaders(headers))
	if err != nil {
		// If creation fails, try to derive (need to recreate headers for GET request)
		headers, err = internal.CreateLevel1Headers(c.web3Client.GetSigner(), nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create level 1 headers for derive: %w", err)
		}
		creds, err = http.Get[types.ApiCreds](c.baseURL, internal.DeriveAPIKey, nil, http.WithHeaders(headers))
		if err != nil {
			return nil, fmt.Errorf("failed to create or derive API creds: %w", err)
		}
	}

	return creds, nil
}
