package websocket

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/polymas/go-polymarket-sdk/internal"
)

type marketSubscriptionRequest struct {
	AssetIDs             []string                `json:"assets_ids"`
	Type                 string                  `json:"type"`
	InitialDump          bool                    `json:"initial_dump"`
	Level                MarketSubscriptionLevel `json:"level"`
	CustomFeatureEnabled bool                    `json:"custom_feature_enabled"`
}

type marketSubscriptionUpdate struct {
	Operation            string                  `json:"operation"`
	AssetIDs             []string                `json:"assets_ids"`
	Level                MarketSubscriptionLevel `json:"level"`
	CustomFeatureEnabled bool                    `json:"custom_feature_enabled"`
}

func (w *webSocketClient) Start(tokenIDs []string) error {
	w.runningMutex.Lock()
	if w.running {
		w.runningMutex.Unlock()
		return fmt.Errorf("Market WebSocket client already running")
	}
	w.stopChan = make(chan struct{})
	w.stopOnce = sync.Once{}
	w.running = true
	stop := w.stopChan
	w.runningMutex.Unlock()

	w.setSubscribedIDs(tokenIDs)
	conn, err := w.connect(stop)
	if err != nil {
		w.runningMutex.Lock()
		if w.stopChan == stop {
			w.running = false
		}
		w.runningMutex.Unlock()
		return err
	}
	go w.run(conn, stop)
	return nil
}

func (w *webSocketClient) Stop() {
	w.runningMutex.Lock()
	if !w.running {
		w.runningMutex.Unlock()
		return
	}
	w.running = false
	stop := w.stopChan
	w.runningMutex.Unlock()

	w.stopOnce.Do(func() { close(stop) })
	if conn := w.currentConn(); conn != nil {
		_ = conn.Close()
	}
}

func (w *webSocketClient) IsRunning() bool {
	w.runningMutex.RLock()
	defer w.runningMutex.RUnlock()
	return w.running
}

func (w *webSocketClient) run(conn *websocket.Conn, stop <-chan struct{}) {
	for {
		_ = w.listen(conn, stop)
		w.clearConn(conn)
		_ = conn.Close()

		if channelClosed(stop) {
			return
		}
		for {
			if !waitForReconnect(stop, w.reconnectDelay) {
				return
			}
			nextConn, err := w.connect(stop)
			if err != nil {
				continue
			}
			conn = nextConn
			break
		}
	}
}

func (w *webSocketClient) connect(stop <-chan struct{}) (*websocket.Conn, error) {
	w.writeMutex.Lock()
	defer w.writeMutex.Unlock()

	netDialer := &net.Dialer{
		Timeout:   internal.WebSocketDialTimeout,
		KeepAlive: internal.WebSocketKeepAlive,
	}
	dialer := websocket.Dialer{
		HandshakeTimeout: internal.WebSocketHandshakeTimeout,
		TLSClientConfig:  &tls.Config{MinVersion: tls.VersionTLS12},
		Proxy:            http.ProxyFromEnvironment,
		NetDial: func(network, addr string) (net.Conn, error) {
			if network == "tcp" {
				network = "tcp4"
			}
			return netDialer.Dial(network, addr)
		},
	}

	conn, _, err := dialer.Dial(w.url, nil)
	if err != nil {
		return nil, fmt.Errorf("connect Market Channel: %w", err)
	}
	request := marketSubscriptionRequest{
		AssetIDs:             w.subscribedIDStrings(),
		Type:                 "market",
		InitialDump:          w.options.InitialDump,
		Level:                w.options.Level,
		CustomFeatureEnabled: w.options.CustomFeatureEnabled,
	}
	if err := conn.WriteJSON(request); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("send Market Channel subscription: %w", err)
	}
	if channelClosed(stop) {
		_ = conn.Close()
		return nil, fmt.Errorf("Market WebSocket client stopped")
	}

	w.connMutex.Lock()
	w.conn = conn
	w.connMutex.Unlock()
	return conn, nil
}

func (w *webSocketClient) listen(conn *websocket.Conn, stop <-chan struct{}) error {
	heartbeatStop := make(chan struct{})
	go w.heartbeat(conn, stop, heartbeatStop)
	defer close(heartbeatStop)

	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		if messageType != websocket.TextMessage || string(message) == "PONG" {
			continue
		}
		w.dispatchMarketMessage(message)
	}
}

func (w *webSocketClient) heartbeat(
	conn *websocket.Conn,
	stop <-chan struct{},
	heartbeatStop <-chan struct{},
) {
	ticker := time.NewTicker(w.heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-heartbeatStop:
			return
		case <-ticker.C:
			w.writeMutex.Lock()
			err := conn.WriteMessage(websocket.TextMessage, []byte("PING"))
			w.writeMutex.Unlock()
			if err != nil {
				_ = conn.Close()
				return
			}
		}
	}
}

func (w *webSocketClient) currentConn() *websocket.Conn {
	w.connMutex.RLock()
	defer w.connMutex.RUnlock()
	return w.conn
}

func (w *webSocketClient) clearConn(conn *websocket.Conn) {
	w.connMutex.Lock()
	if w.conn == conn {
		w.conn = nil
	}
	w.connMutex.Unlock()
}
