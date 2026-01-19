package websocket

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/polymas/go-polymarket-sdk/internal"
)

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

// connectAndListenUserChannel 连接并监听 USER 频道
func (w *webSocketClient) connectAndListenUserChannel() error {
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

	conn, _, err := dialer.Dial(wsUserURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to USER channel: %w", err)
	}

	w.userConnMutex.Lock()
	w.userConn = conn
	w.userConnMutex.Unlock()

	// Send authentication and subscription message
	subMsg := map[string]interface{}{
		"type": "USER",
		"auth": w.auth,
	}

	if err := conn.WriteJSON(subMsg); err != nil {
		return fmt.Errorf("failed to send USER channel subscription: %w", err)
	}

	// Start heartbeat
	heartbeatStop := make(chan struct{})
	go w.heartbeatUserChannel(conn, heartbeatStop)
	defer close(heartbeatStop)

	// Listen for messages
	for {
		select {
		case <-w.userStopChan:
			return nil
		default:
			messageType, messageBytes, err := conn.ReadMessage()
			if err != nil {
				return fmt.Errorf("failed to read USER channel message: %w", err)
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

				// Handle order updates
				if eventType, ok := msg["event_type"].(string); ok {
					switch eventType {
					case "order":
						w.handleOrderUpdate(msg)
					case "trade":
						w.handleTradeUpdate(msg)
					}
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

// heartbeatUserChannel sends periodic PING messages for USER channel
func (w *webSocketClient) heartbeatUserChannel(conn *websocket.Conn, stop chan struct{}) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if err := conn.WriteJSON("PING"); err != nil {
				return
			}
		}
	}
}
