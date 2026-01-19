package client

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/polymarket/go-polymarket-sdk/internal"
	"github.com/polymarket/go-polymarket-sdk/types"
)

const (
	wsMarketURL = "wss://ws-subscriptions-clob.polymarket.com/ws/market"
)

// WebSocketAuth 表示WebSocket连接的认证信息
// According to Polymarket WSS documentation: https://docs.polymarket.com/developers/CLOB/websocket/wss-overview
type WebSocketAuth struct {
	Address   string `json:"address,omitempty"`
	Signature string `json:"signature,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	Nonce     string `json:"nonce,omitempty"`
}

// WebSocketClient 定义WebSocket客户端的接口，供外部包使用
type WebSocketClient interface {
	SetOnBookUpdate(callback func(assetID string, snapshot *types.BookSnapshot))
	SetAuth(auth *WebSocketAuth)
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
	auth            *WebSocketAuth // Optional authentication (for authenticated channels)
	lastConnected   time.Time      // 最后连接成功的时间
	disconnectedAt  *time.Time     // 断连时间（nil表示已连接）
	disconnectMutex sync.RWMutex   // 保护断连时间
}

// NewWebSocketClient 创建新的WebSocket客户端
// 返回 WebSocketClient 接口，不允许直接访问实现类型
func NewWebSocketClient(reconnectDelay time.Duration) WebSocketClient {
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

// SetAuth 设置WebSocket连接的认证信息
// 这是可选的 - MARKET频道可能无需认证，但USER频道需要认证
func (w *webSocketClient) SetAuth(auth *WebSocketAuth) {
	w.auth = auth
}

// Start 启动WebSocket连接和订阅循环
func (w *webSocketClient) Start(assetIDs []string) error {
	w.runningMutex.Lock()
	if w.running {
		w.runningMutex.Unlock()
		return fmt.Errorf("WebSocket client already running")
	}

	// Always create a new stopChan for each Start() call
	// This allows restarting after Stop() without panic
	w.stopChan = make(chan struct{})
	w.stopOnce = sync.Once{} // Reset sync.Once for new run

	w.running = true
	w.runningMutex.Unlock()

	w.subscribedMutex.Lock()
	w.subscribedIDs = assetIDs
	w.subscribedMutex.Unlock()
	go w.run()
	return nil
}

// Stop 停止WebSocket客户端
func (w *webSocketClient) Stop() {
	w.runningMutex.Lock()
	if !w.running {
		w.runningMutex.Unlock()
		return
	}
	w.running = false
	w.runningMutex.Unlock()

	// Use sync.Once to ensure stopChan is only closed once
	w.stopOnce.Do(func() {
		close(w.stopChan)
	})

	w.connMutex.Lock()
	if w.conn != nil {
		w.conn.Close()
	}
	w.connMutex.Unlock()
}

// IsRunning 返回客户端是否正在运行
func (w *webSocketClient) IsRunning() bool {
	w.runningMutex.RLock()
	defer w.runningMutex.RUnlock()
	return w.running
}

// UpdateSubscription 使用新的资产ID更新订阅
// 这会发送包含新资产列表的订阅消息
func (w *webSocketClient) UpdateSubscription(assetIDs []string) error {
	w.subscribedMutex.Lock()
	w.subscribedIDs = assetIDs
	w.subscribedMutex.Unlock()
	w.connMutex.Lock()
	conn := w.conn
	w.connMutex.Unlock()

	if conn != nil {
		// Send new subscription message
		subMsg := map[string]interface{}{
			"assets_ids": assetIDs,
			"type":       "MARKET",
		}
		if w.auth != nil {
			subMsg["auth"] = w.auth
		}
		// 订阅更新日志已移除
		return conn.WriteJSON(subMsg)
	}
	return nil
}

// SubscribeAssets 订阅额外的资产（动态订阅）
// 根据Polymarket文档：https://docs.polymarket.com/developers/CLOB/websocket/wss-overview
// 这允许在初始连接后订阅更多资产
func (w *webSocketClient) SubscribeAssets(assetIDs []string) error {
	w.connMutex.Lock()
	conn := w.conn
	w.connMutex.Unlock()

	if conn == nil {
		return fmt.Errorf("WebSocket not connected")
	}

	// Add to subscribed list
	w.subscribedMutex.Lock()
	w.subscribedIDs = append(w.subscribedIDs, assetIDs...)
	w.subscribedMutex.Unlock()

	// Send subscribe message
	subMsg := map[string]interface{}{
		"assets_ids": assetIDs,
		"operation":  "subscribe",
	}
	if w.auth != nil {
		subMsg["auth"] = w.auth
	}

	// 动态订阅日志已移除
	return conn.WriteJSON(subMsg)
}

// UnsubscribeAssets 取消订阅资产（动态取消订阅）
func (w *webSocketClient) UnsubscribeAssets(assetIDs []string) error {
	w.connMutex.Lock()
	conn := w.conn
	w.connMutex.Unlock()

	if conn == nil {
		return fmt.Errorf("WebSocket not connected")
	}

	// Remove from subscribed list
	assetMap := make(map[string]bool)
	for _, id := range assetIDs {
		assetMap[id] = true
	}

	w.subscribedMutex.Lock()
	newSubscribed := []string{}
	for _, id := range w.subscribedIDs {
		if !assetMap[id] {
			newSubscribed = append(newSubscribed, id)
		}
	}
	w.subscribedIDs = newSubscribed
	w.subscribedMutex.Unlock()

	// Send unsubscribe message
	unsubMsg := map[string]interface{}{
		"assets_ids": assetIDs,
		"operation":  "unsubscribe",
	}
	if w.auth != nil {
		unsubMsg["auth"] = w.auth
	}

	// 动态取消订阅日志已移除
	return conn.WriteJSON(unsubMsg)
}

// run runs the main WebSocket loop
func (w *webSocketClient) run() {
	// 启动定时打印订阅资产总数的goroutine
	go w.logSubscriptionStats()

	for {
		select {
		case <-w.stopChan:
			return
		default:
			// 记录断连时间
			now := time.Now()
			w.disconnectMutex.Lock()
			if w.disconnectedAt == nil {
				w.disconnectedAt = &now
			}
			w.disconnectMutex.Unlock()

			if err := w.connectAndListen(); err != nil {
				// 检查断连时间，超过1分钟才输出日志
				w.disconnectMutex.RLock()
				disconnectedAt := w.disconnectedAt
				w.disconnectMutex.RUnlock()

				if disconnectedAt != nil {
					disconnectedDuration := time.Since(*disconnectedAt)
					if disconnectedDuration > 1*time.Minute {
						log.Printf("[WARN] [WebSocket] 断连超过1分钟 (已断连: %v), 错误: %v", disconnectedDuration, err)
					}
				}

				time.Sleep(w.reconnectDelay)
			} else {
				// 连接成功，清除断连时间
				w.disconnectMutex.Lock()
				w.disconnectedAt = nil
				w.lastConnected = time.Now()
				w.disconnectMutex.Unlock()
			}
		}
	}
}

// logSubscriptionStats 每10秒打印一次订阅的资产总数
func (w *webSocketClient) logSubscriptionStats() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopChan:
			return
		case <-ticker.C:
			// 获取订阅的资产总数（加锁保护）
			w.subscribedMutex.RLock()
			assetCount := len(w.subscribedIDs)
			w.subscribedMutex.RUnlock()
			internal.LogDebug("[WebSocket] 当前订阅的资产总数: %d", assetCount)
		}
	}
}

// connectAndListen connects to WebSocket and listens for messages
func (w *webSocketClient) connectAndListen() error {
	// Configure dialer to prefer IPv4 and increase timeout
	netDialer := &net.Dialer{
		Timeout:   internal.WebSocketDialTimeout,
		DualStack: false, // Disable IPv6 to avoid timeout issues
		KeepAlive: internal.WebSocketKeepAlive,
	}

	// Configure TLS
	tlsConfig := &tls.Config{
		InsecureSkipVerify: false, // 验证SSL证书
	}

	// Check for proxy from environment variables
	var proxyURL *url.URL
	if proxyEnv := os.Getenv("HTTPS_PROXY"); proxyEnv != "" {
		if proxyEnv == "" {
			proxyEnv = os.Getenv("HTTP_PROXY")
		}
		if proxyEnv != "" {
			proxyURL, _ = url.Parse(proxyEnv)
		}
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: internal.WebSocketHandshakeTimeout,
		TLSClientConfig:  tlsConfig,
		Proxy: func(req *http.Request) (*url.URL, error) {
			if proxyURL != nil {
				return proxyURL, nil
			}
			return http.ProxyFromEnvironment(req)
		},
		NetDial: func(network, addr string) (net.Conn, error) {
			// Force IPv4 by using "tcp4" instead of "tcp"
			if network == "tcp" {
				network = "tcp4"
			}
			// TCP连接日志已移除
			conn, err := netDialer.Dial(network, addr)
			return conn, err
		},
	}

	// WebSocket连接日志已移除
	conn, _, err := dialer.Dial(w.url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	w.connMutex.Lock()
	w.conn = conn
	w.connMutex.Unlock()

	// Send subscription message according to Polymarket WSS documentation
	// https://docs.polymarket.com/developers/CLOB/websocket/wss-overview
	w.subscribedMutex.RLock()
	subscribedIDsCopy := make([]string, len(w.subscribedIDs))
	copy(subscribedIDsCopy, w.subscribedIDs)
	w.subscribedMutex.RUnlock()

	if len(subscribedIDsCopy) > 0 {
		subMsg := map[string]interface{}{
			"assets_ids": subscribedIDsCopy,
			"type":       "MARKET",
		}

		// Add auth field if provided (optional for MARKET channel, required for USER channel)
		if w.auth != nil {
			subMsg["auth"] = w.auth
		}

		if err := conn.WriteJSON(subMsg); err != nil {
			return fmt.Errorf("failed to send subscription: %w", err)
		}
	}

	// Start heartbeat goroutine
	heartbeatStop := make(chan struct{})
	go w.heartbeat(conn, heartbeatStop)
	defer close(heartbeatStop)

	// Listen for messages
	for {
		select {
		case <-w.stopChan:
			return nil
		default:
			// Read message as raw bytes first to handle non-JSON messages (like "PONG")
			messageType, messageBytes, err := conn.ReadMessage()
			if err != nil {
				// 记录断连时间
				now := time.Now()
				w.disconnectMutex.Lock()
				if w.disconnectedAt == nil {
					w.disconnectedAt = &now
				}
				w.disconnectMutex.Unlock()
				return fmt.Errorf("failed to read message: %w", err)
			}

			// Handle text messages only (ignore binary)
			if messageType != websocket.TextMessage {
				continue
			}

			// Check for plain text PONG message (not JSON)
			messageStr := string(messageBytes)
			if messageStr == "PONG" {
				continue
			}

			// Parse JSON message (can be object or array)
			var rawMsg interface{}
			if err := json.Unmarshal(messageBytes, &rawMsg); err != nil {
				continue
			}

			// Normalize to list (handle both array and single object)
			var msgList []interface{}
			switch v := rawMsg.(type) {
			case []interface{}:
				msgList = v
			case map[string]interface{}:
				msgList = []interface{}{v}
			default:
				continue
			}

			// Process each message in the list
			for _, rawData := range msgList {
				msg, ok := rawData.(map[string]interface{})
				if !ok {
					continue
				}

				// Handle PONG
				if msgStr, ok := msg["type"].(string); ok && msgStr == "PONG" {
					continue
				}

				// Handle book updates
				if eventType, ok := msg["event_type"].(string); ok && eventType == "book" {
					w.handleBookUpdate(msg)
				}
			}
		}
	}
}

// heartbeat sends periodic PING messages
func (w *webSocketClient) heartbeat(conn *websocket.Conn, stop chan struct{}) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if err := conn.WriteJSON("PING"); err != nil {
				// 记录断连时间
				now := time.Now()
				w.disconnectMutex.Lock()
				if w.disconnectedAt == nil {
					w.disconnectedAt = &now
				}
				w.disconnectMutex.Unlock()
				return
			}
		}
	}
}

// handleBookUpdate handles order book update messages
func (w *webSocketClient) handleBookUpdate(msg map[string]interface{}) {
	assetID, ok := msg["asset_id"].(string)
	if !ok {
		return
	}

	// Clean asset ID
	assetID = strings.TrimPrefix(assetID, "0x")

	var bestBid, bestAsk *types.BookSide

	// Parse bids - Python code: bb = self._parse_top(bids, max)
	// Python finds the maximum price from all bids
	if bidsRaw, ok := msg["bids"]; ok && bidsRaw != nil {
		var bids []interface{}
		switch v := bidsRaw.(type) {
		case []interface{}:
			bids = v
		default:
			// Skip invalid bid format
		}

		if len(bids) > 0 {
			// Find best bid (highest price) - Python uses max(parsed, key=lambda x: x[0])
			var bestPrice float64 = -1
			var bestSize float64 = 0
			for _, bidRaw := range bids {
				bid, ok := bidRaw.(map[string]interface{})
				if !ok {
					continue
				}
				price := parseFloat64(bid["price"])
				size := parseFloat64(bid["size"])
				if price > bestPrice {
					bestPrice = price
					bestSize = size
				}
			}
			if bestPrice > 0 {
				bestBid = &types.BookSide{Price: bestPrice, Size: bestSize}
			}
		}
	}

	// Parse asks - Python code: ba = self._parse_top(asks, min)
	// Python finds the minimum price from all asks
	if asksRaw, ok := msg["asks"]; ok && asksRaw != nil {
		var asks []interface{}
		switch v := asksRaw.(type) {
		case []interface{}:
			asks = v
		default:
			// Skip invalid ask format
		}

		if len(asks) > 0 {
			// Find best ask (lowest price) - Python uses min(parsed, key=lambda x: x[0])
			var bestPrice float64 = -1
			var bestSize float64 = 0
			for _, askRaw := range asks {
				ask, ok := askRaw.(map[string]interface{})
				if !ok {
					continue
				}
				price := parseFloat64(ask["price"])
				size := parseFloat64(ask["size"])
				if bestPrice < 0 || price < bestPrice {
					bestPrice = price
					bestSize = size
				}
			}
			if bestPrice > 0 {
				bestAsk = &types.BookSide{Price: bestPrice, Size: bestSize}
			}
		}
	}

	snapshot := &types.BookSnapshot{
		BestBid: bestBid,
		BestAsk: bestAsk,
		TS:      time.Now(),
	}

	// Debug日志：记录订单簿更新
	internal.LogDebug("[WebSocket] 收到订单簿更新: asset_id=%s, best_bid=%v, best_ask=%v",
		assetID, bestBid, bestAsk)

	if w.onBookUpdate != nil {
		w.onBookUpdate(assetID, snapshot)
	}
}

// parseFloat64 safely parses a float64 from interface{}
func parseFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		var f float64
		fmt.Sscanf(val, "%f", &f)
		return f
	default:
		return 0
	}
}
