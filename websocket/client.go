package websocket

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/polymas/go-polymarket-sdk/types"
)

const (
	wsMarketURL = "wss://ws-subscriptions-clob.polymarket.com/ws/market"
)

// Client 定义公开 Market WebSocket 客户端。
// User Channel 使用独立的 UserClient。
type Client interface {
	SetOnBookUpdate(callback func(assetID string, snapshot *types.BookSnapshot))
	Start(assetIDs []string) error
	Stop()
	IsRunning() bool
	UpdateSubscription(assetIDs []string) error
	SubscribeAssets(assetIDs []string) error
	UnsubscribeAssets(assetIDs []string) error
}

// webSocketClient 处理订单簿订阅的WebSocket连接
// 不允许直接导出，只能通过 NewWebSocketClient 创建
type webSocketClient struct {
	url             string
	conn            *websocket.Conn
	connMutex       sync.RWMutex
	subscribedIDs   []string
	subscribedMutex sync.RWMutex // 保护subscribedIDs的访问
	onBookUpdate    func(assetID string, snapshot *types.BookSnapshot)
	reconnectDelay  time.Duration
	stopChan        chan struct{}
	stopOnce        sync.Once // Ensure stopChan is only closed once
	running         bool
	runningMutex    sync.RWMutex
	lastConnected   time.Time    // 最后连接成功的时间
	disconnectedAt  *time.Time   // 断连时间（nil表示已连接）
	disconnectMutex sync.RWMutex // 保护断连时间
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
