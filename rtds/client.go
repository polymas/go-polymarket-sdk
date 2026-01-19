package rtds

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

const (
	rtdsURL = "wss://rtds.polymarket.com/ws"
)

// Client 定义 RTDS 客户端的接口
type Client interface {
	SetOnPriceUpdate(callback func(update *types.EncryptedPriceUpdate))
	SetOnCommentUpdate(callback func(comment *types.Comment))
	SetAuth(auth *RTDSAuth)
	Start() error
	Stop()
	IsRunning() bool
	SubscribePrices(tokenIDs []string) error
	UnsubscribePrices(tokenIDs []string) error
	SubscribeComments(marketIDs []string) error
	UnsubscribeComments(marketIDs []string) error
}

// RTDSAuth 表示 RTDS 连接的认证信息
type RTDSAuth struct {
	Token string `json:"token"`
}

// rtdsClient 处理 RTDS WebSocket 连接
type rtdsClient struct {
	conn                *websocket.Conn
	connMutex           sync.RWMutex
	onPriceUpdate       func(update *types.EncryptedPriceUpdate)
	onCommentUpdate     func(comment *types.Comment)
	reconnectDelay      time.Duration
	stopChan            chan struct{}
	stopOnce            sync.Once
	running             bool
	runningMutex        sync.RWMutex
	auth                *RTDSAuth
	subscribedPrices    map[string]bool
	subscribedComments  map[string]bool
	subscriptionsMutex  sync.RWMutex
	lastConnected       time.Time
	disconnectedAt      *time.Time
	disconnectMutex     sync.RWMutex
}

// NewClient 创建新的 RTDS 客户端
func NewClient(reconnectDelay time.Duration) Client {
	return &rtdsClient{
		reconnectDelay:     reconnectDelay,
		stopChan:           make(chan struct{}),
		subscribedPrices:   make(map[string]bool),
		subscribedComments: make(map[string]bool),
	}
}

// SetOnPriceUpdate 设置价格更新的回调函数
func (r *rtdsClient) SetOnPriceUpdate(callback func(update *types.EncryptedPriceUpdate)) {
	r.onPriceUpdate = callback
}

// SetOnCommentUpdate 设置评论更新的回调函数
func (r *rtdsClient) SetOnCommentUpdate(callback func(comment *types.Comment)) {
	r.onCommentUpdate = callback
}

// SetAuth 设置认证信息
func (r *rtdsClient) SetAuth(auth *RTDSAuth) {
	r.auth = auth
}

// Start 启动 RTDS WebSocket 连接
func (r *rtdsClient) Start() error {
	r.runningMutex.Lock()
	if r.running {
		r.runningMutex.Unlock()
		return fmt.Errorf("RTDS client already running")
	}

	r.stopChan = make(chan struct{})
	r.stopOnce = sync.Once{}
	r.running = true
	r.runningMutex.Unlock()

	go r.run()
	return nil
}

// Stop 停止 RTDS 客户端
func (r *rtdsClient) Stop() {
	r.runningMutex.Lock()
	if !r.running {
		r.runningMutex.Unlock()
		return
	}
	r.running = false
	r.runningMutex.Unlock()

	r.stopOnce.Do(func() {
		close(r.stopChan)
	})

	r.connMutex.Lock()
	if r.conn != nil {
		r.conn.Close()
	}
	r.connMutex.Unlock()
}

// IsRunning 返回客户端是否正在运行
func (r *rtdsClient) IsRunning() bool {
	r.runningMutex.RLock()
	defer r.runningMutex.RUnlock()
	return r.running
}

// SubscribePrices 订阅价格流
func (r *rtdsClient) SubscribePrices(tokenIDs []string) error {
	r.subscriptionsMutex.Lock()
	for _, tokenID := range tokenIDs {
		r.subscribedPrices[tokenID] = true
	}
	r.subscriptionsMutex.Unlock()

	return r.sendSubscription("prices", tokenIDs)
}

// UnsubscribePrices 取消订阅价格流
func (r *rtdsClient) UnsubscribePrices(tokenIDs []string) error {
	r.subscriptionsMutex.Lock()
	for _, tokenID := range tokenIDs {
		delete(r.subscribedPrices, tokenID)
	}
	r.subscriptionsMutex.Unlock()

	return r.sendUnsubscription("prices", tokenIDs)
}

// SubscribeComments 订阅评论流
func (r *rtdsClient) SubscribeComments(marketIDs []string) error {
	r.subscriptionsMutex.Lock()
	for _, marketID := range marketIDs {
		r.subscribedComments[marketID] = true
	}
	r.subscriptionsMutex.Unlock()

	return r.sendSubscription("comments", marketIDs)
}

// UnsubscribeComments 取消订阅评论流
func (r *rtdsClient) UnsubscribeComments(marketIDs []string) error {
	r.subscriptionsMutex.Lock()
	for _, marketID := range marketIDs {
		delete(r.subscribedComments, marketID)
	}
	r.subscriptionsMutex.Unlock()

	return r.sendUnsubscription("comments", marketIDs)
}

// run 运行主循环
func (r *rtdsClient) run() {
	for {
		select {
		case <-r.stopChan:
			return
		default:
			now := time.Now()
			r.disconnectMutex.Lock()
			if r.disconnectedAt == nil {
				r.disconnectedAt = &now
			}
			r.disconnectMutex.Unlock()

			if err := r.connectAndListen(); err != nil {
				time.Sleep(r.reconnectDelay)
			} else {
				r.disconnectMutex.Lock()
				r.disconnectedAt = nil
				r.lastConnected = time.Now()
				r.disconnectMutex.Unlock()
			}
		}
	}
}

// connectAndListen 连接并监听消息
func (r *rtdsClient) connectAndListen() error {
	netDialer := &net.Dialer{
		Timeout:   internal.WebSocketDialTimeout,
		DualStack: false,
		KeepAlive: internal.WebSocketKeepAlive,
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: false,
	}

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
			if network == "tcp" {
				network = "tcp4"
			}
			return netDialer.Dial(network, addr)
		},
	}

	conn, _, err := dialer.Dial(rtdsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	r.connMutex.Lock()
	r.conn = conn
	r.connMutex.Unlock()

	// Send authentication if provided
	if r.auth != nil {
		authMsg := map[string]interface{}{
			"type":  "auth",
			"token": r.auth.Token,
		}
		if err := conn.WriteJSON(authMsg); err != nil {
			return fmt.Errorf("failed to send auth: %w", err)
		}
	}

	// Resubscribe to existing subscriptions
	r.subscriptionsMutex.RLock()
	priceIDs := make([]string, 0, len(r.subscribedPrices))
	for tokenID := range r.subscribedPrices {
		priceIDs = append(priceIDs, tokenID)
	}
	commentIDs := make([]string, 0, len(r.subscribedComments))
	for marketID := range r.subscribedComments {
		commentIDs = append(commentIDs, marketID)
	}
	r.subscriptionsMutex.RUnlock()

	if len(priceIDs) > 0 {
		if err := r.sendSubscription("prices", priceIDs); err != nil {
			return err
		}
	}
	if len(commentIDs) > 0 {
		if err := r.sendSubscription("comments", commentIDs); err != nil {
			return err
		}
	}

	// Start heartbeat
	heartbeatStop := make(chan struct{})
	go r.heartbeat(conn, heartbeatStop)
	defer close(heartbeatStop)

	// Listen for messages
	for {
		select {
		case <-r.stopChan:
			return nil
		default:
			messageType, messageBytes, err := conn.ReadMessage()
			if err != nil {
				now := time.Now()
				r.disconnectMutex.Lock()
				if r.disconnectedAt == nil {
					r.disconnectedAt = &now
				}
				r.disconnectMutex.Unlock()
				return fmt.Errorf("failed to read message: %w", err)
			}

			if messageType != websocket.TextMessage {
				continue
			}

			messageStr := string(messageBytes)
			if messageStr == "PONG" {
				continue
			}

			var rawMsg interface{}
			if err := json.Unmarshal(messageBytes, &rawMsg); err != nil {
				continue
			}

			var msgList []interface{}
			switch v := rawMsg.(type) {
			case []interface{}:
				msgList = v
			case map[string]interface{}:
				msgList = []interface{}{v}
			default:
				continue
			}

			for _, rawData := range msgList {
				msg, ok := rawData.(map[string]interface{})
				if !ok {
					continue
				}

				if msgStr, ok := msg["type"].(string); ok && msgStr == "PONG" {
					continue
				}

				// Handle price updates
				if streamType, ok := msg["stream"].(string); ok {
					switch streamType {
					case "prices":
						r.handlePriceUpdate(msg)
					case "comments":
						r.handleCommentUpdate(msg)
					}
				}
			}
		}
	}
}

// heartbeat 发送定期 PING 消息
func (r *rtdsClient) heartbeat(conn *websocket.Conn, stop chan struct{}) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if err := conn.WriteJSON("PING"); err != nil {
				now := time.Now()
				r.disconnectMutex.Lock()
				if r.disconnectedAt == nil {
					r.disconnectedAt = &now
				}
				r.disconnectMutex.Unlock()
				return
			}
		}
	}
}

// sendSubscription 发送订阅消息
func (r *rtdsClient) sendSubscription(streamType string, ids []string) error {
	r.connMutex.RLock()
	conn := r.conn
	r.connMutex.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	subMsg := map[string]interface{}{
		"type":  "subscribe",
		"stream": streamType,
		"ids":   ids,
	}

	return conn.WriteJSON(subMsg)
}

// sendUnsubscription 发送取消订阅消息
func (r *rtdsClient) sendUnsubscription(streamType string, ids []string) error {
	r.connMutex.RLock()
	conn := r.conn
	r.connMutex.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	unsubMsg := map[string]interface{}{
		"type":  "unsubscribe",
		"stream": streamType,
		"ids":   ids,
	}

	return conn.WriteJSON(unsubMsg)
}

// handlePriceUpdate 处理价格更新
func (r *rtdsClient) handlePriceUpdate(msg map[string]interface{}) {
	if r.onPriceUpdate == nil {
		return
	}

	updateJSON, err := json.Marshal(msg)
	if err != nil {
		return
	}

	var update types.EncryptedPriceUpdate
	if err := json.Unmarshal(updateJSON, &update); err != nil {
		return
	}

	r.onPriceUpdate(&update)
}

// handleCommentUpdate 处理评论更新
func (r *rtdsClient) handleCommentUpdate(msg map[string]interface{}) {
	if r.onCommentUpdate == nil {
		return
	}

	commentJSON, err := json.Marshal(msg)
	if err != nil {
		return
	}

	var comment types.Comment
	if err := json.Unmarshal(commentJSON, &comment); err != nil {
		return
	}

	r.onCommentUpdate(&comment)
}
