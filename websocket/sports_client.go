package websocket

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
	wsSportsURL = "wss://ws-subscriptions-clob.polymarket.com/ws/sports"
)

// SportsClient 定义 Sports WebSocket 客户端的接口
type SportsClient interface {
	SetOnSportsUpdate(callback func(event *types.SportsEvent))
	SetAuth(auth *WebSocketAuth)
	Start() error
	Stop()
	IsRunning() bool
}

// sportsWebSocketClient 处理体育市场订阅的 WebSocket 连接
type sportsWebSocketClient struct {
	conn            *websocket.Conn
	connMutex       sync.RWMutex
	onSportsUpdate  func(event *types.SportsEvent)
	reconnectDelay  time.Duration
	stopChan        chan struct{}
	stopOnce        sync.Once
	running         bool
	runningMutex    sync.RWMutex
	auth            *WebSocketAuth
	lastConnected   time.Time
	disconnectedAt  *time.Time
	disconnectMutex sync.RWMutex
}

// NewSportsClient 创建新的 Sports WebSocket 客户端
func NewSportsClient(reconnectDelay time.Duration) SportsClient {
	return &sportsWebSocketClient{
		reconnectDelay: reconnectDelay,
		stopChan:       make(chan struct{}),
	}
}

// SetOnSportsUpdate 设置体育事件更新的回调函数
func (s *sportsWebSocketClient) SetOnSportsUpdate(callback func(event *types.SportsEvent)) {
	s.onSportsUpdate = callback
}

// SetAuth 设置认证信息
func (s *sportsWebSocketClient) SetAuth(auth *WebSocketAuth) {
	s.auth = auth
}

// Start 启动 Sports WebSocket 连接
func (s *sportsWebSocketClient) Start() error {
	s.runningMutex.Lock()
	if s.running {
		s.runningMutex.Unlock()
		return fmt.Errorf("Sports WebSocket client already running")
	}

	s.stopChan = make(chan struct{})
	s.stopOnce = sync.Once{}
	s.running = true
	s.runningMutex.Unlock()

	go s.run()
	return nil
}

// Stop 停止 Sports WebSocket 客户端
func (s *sportsWebSocketClient) Stop() {
	s.runningMutex.Lock()
	if !s.running {
		s.runningMutex.Unlock()
		return
	}
	s.running = false
	s.runningMutex.Unlock()

	s.stopOnce.Do(func() {
		close(s.stopChan)
	})

	s.connMutex.Lock()
	if s.conn != nil {
		s.conn.Close()
	}
	s.connMutex.Unlock()
}

// IsRunning 返回客户端是否正在运行
func (s *sportsWebSocketClient) IsRunning() bool {
	s.runningMutex.RLock()
	defer s.runningMutex.RUnlock()
	return s.running
}

// run 运行主循环
func (s *sportsWebSocketClient) run() {
	for {
		select {
		case <-s.stopChan:
			return
		default:
			now := time.Now()
			s.disconnectMutex.Lock()
			if s.disconnectedAt == nil {
				s.disconnectedAt = &now
			}
			s.disconnectMutex.Unlock()

			if err := s.connectAndListen(); err != nil {
				time.Sleep(s.reconnectDelay)
			} else {
				s.disconnectMutex.Lock()
				s.disconnectedAt = nil
				s.lastConnected = time.Now()
				s.disconnectMutex.Unlock()
			}
		}
	}
}

// connectAndListen 连接并监听消息
func (s *sportsWebSocketClient) connectAndListen() error {
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

	conn, _, err := dialer.Dial(wsSportsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	s.connMutex.Lock()
	s.conn = conn
	s.connMutex.Unlock()

	// Send subscription message
	subMsg := map[string]interface{}{
		"type": "SPORTS",
	}
	if s.auth != nil {
		subMsg["auth"] = s.auth
	}

	if err := conn.WriteJSON(subMsg); err != nil {
		return fmt.Errorf("failed to send subscription: %w", err)
	}

	// Start heartbeat
	heartbeatStop := make(chan struct{})
	go s.heartbeat(conn, heartbeatStop)
	defer close(heartbeatStop)

	// Listen for messages
	for {
		select {
		case <-s.stopChan:
			return nil
		default:
			messageType, messageBytes, err := conn.ReadMessage()
			if err != nil {
				now := time.Now()
				s.disconnectMutex.Lock()
				if s.disconnectedAt == nil {
					s.disconnectedAt = &now
				}
				s.disconnectMutex.Unlock()
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

				// Handle sports updates
				if eventType, ok := msg["event_type"].(string); ok && eventType == "sports" {
					s.handleSportsUpdate(msg)
				}
			}
		}
	}
}

// heartbeat 发送定期 PING 消息
func (s *sportsWebSocketClient) heartbeat(conn *websocket.Conn, stop chan struct{}) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if err := conn.WriteJSON("PING"); err != nil {
				now := time.Now()
				s.disconnectMutex.Lock()
				if s.disconnectedAt == nil {
					s.disconnectedAt = &now
				}
				s.disconnectMutex.Unlock()
				return
			}
		}
	}
}

// handleSportsUpdate 处理体育事件更新
func (s *sportsWebSocketClient) handleSportsUpdate(msg map[string]interface{}) {
	if s.onSportsUpdate == nil {
		return
	}

	eventJSON, err := json.Marshal(msg)
	if err != nil {
		return
	}

	var event types.SportsEvent
	if err := json.Unmarshal(eventJSON, &event); err != nil {
		return
	}

	s.onSportsUpdate(&event)
}
