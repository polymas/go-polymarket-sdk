package websocket

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/polymas/go-polymarket-sdk/types"
)

const (
	wsMarketURL = "wss://ws-subscriptions-clob.polymarket.com/ws/market"
	wsUserURL   = "wss://ws-subscriptions-clob.polymarket.com/ws/user"
)

// WebSocketAuth 表示WebSocket连接的认证信息
// According to Polymarket WSS documentation: https://docs.polymarket.com/developers/CLOB/websocket/wss-overview
type WebSocketAuth struct {
	Address   string `json:"address,omitempty"`
	Signature string `json:"signature,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	Nonce     string `json:"nonce,omitempty"`
}

// Client 定义WebSocket客户端的接口，供外部包使用
type Client interface {
	SetOnBookUpdate(callback func(assetID string, snapshot *types.BookSnapshot))
	SetOnOrderUpdate(callback func(order *types.OpenOrder))
	SetOnTradeUpdate(callback func(trade *types.PolygonTrade))
	SetAuth(auth *WebSocketAuth)
	Start(assetIDs []string) error
	Stop()
	IsRunning() bool
	UpdateSubscription(assetIDs []string) error
	SubscribeAssets(assetIDs []string) error
	UnsubscribeAssets(assetIDs []string) error
	// USER 频道方法
	StartUserChannel() error
	StopUserChannel()
}

// webSocketClient 处理订单簿订阅的WebSocket连接
// 不允许直接导出，只能通过 NewWebSocketClient 创建
type webSocketClient struct {
	url              string
	conn             *websocket.Conn
	connMutex        sync.RWMutex
	subscribedIDs    []string
	subscribedMutex  sync.RWMutex // 保护subscribedIDs的访问
	onBookUpdate     func(assetID string, snapshot *types.BookSnapshot)
	onOrderUpdate    func(order *types.OpenOrder)
	onTradeUpdate    func(trade *types.PolygonTrade)
	reconnectDelay   time.Duration
	stopChan         chan struct{}
	stopOnce         sync.Once // Ensure stopChan is only closed once
	running          bool
	runningMutex     sync.RWMutex
	auth             *WebSocketAuth // Optional authentication (for authenticated channels)
	lastConnected    time.Time      // 最后连接成功的时间
	disconnectedAt   *time.Time     // 断连时间（nil表示已连接）
	disconnectMutex  sync.RWMutex   // 保护断连时间
	// USER 频道相关
	userConn         *websocket.Conn
	userConnMutex    sync.RWMutex
	userRunning      bool
	userRunningMutex sync.RWMutex
	userStopChan     chan struct{}
	userStopOnce     sync.Once
}

// NewClient 创建新的WebSocket客户端
// 返回 Client 接口，不允许直接访问实现类型
func NewClient(reconnectDelay time.Duration) Client {
	return &webSocketClient{
		url:            wsMarketURL,
		reconnectDelay: reconnectDelay,
		stopChan:       make(chan struct{}),
	}
}

// SetOnBookUpdate 设置订单簿更新的回调函数
func (w *webSocketClient) SetOnBookUpdate(callback func(assetID string, snapshot *types.BookSnapshot)) {
	w.onBookUpdate = callback
}

// SetOnOrderUpdate 设置订单状态更新的回调函数（USER 频道）
func (w *webSocketClient) SetOnOrderUpdate(callback func(order *types.OpenOrder)) {
	w.onOrderUpdate = callback
}

// SetOnTradeUpdate 设置交易更新的回调函数（USER 频道）
func (w *webSocketClient) SetOnTradeUpdate(callback func(trade *types.PolygonTrade)) {
	w.onTradeUpdate = callback
}

// SetAuth 设置WebSocket连接的认证信息
// 这是可选的 - MARKET频道可能无需认证，但USER频道需要认证
func (w *webSocketClient) SetAuth(auth *WebSocketAuth) {
	w.auth = auth
}
