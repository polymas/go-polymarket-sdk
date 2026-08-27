package websocket

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/polymas/go-polymarket-sdk/internal"
)

const wsSportsURL = "wss://sports-api.polymarket.com/ws"

// SportResult is a real-time match result from the public Sports Channel.
// Only Slug is required by the protocol; omitted optional fields keep their
// zero value. Timestamp strings are preserved exactly as sent by Polymarket.
type SportResult struct {
	Slug              string `json:"slug"`
	Live              bool   `json:"live"`
	Ended             bool   `json:"ended"`
	Score             string `json:"score"`
	Period            string `json:"period"`
	Elapsed           string `json:"elapsed"`
	LastUpdate        string `json:"last_update"`
	FinishedTimestamp string `json:"finished_timestamp"`
	Turn              string `json:"turn"`
}

// SportsClient receives public match updates. The channel needs neither
// authentication nor a subscription message.
type SportsClient interface {
	SetOnSportsUpdate(callback func(result *SportResult))
	Start() error
	Stop()
	IsRunning() bool
}

type sportsWebSocketClient struct {
	url            string
	reconnectDelay time.Duration

	conn       *websocket.Conn
	connMutex  sync.RWMutex
	writeMutex sync.Mutex

	onSportsUpdate func(result *SportResult)
	callbackMutex  sync.RWMutex

	stopChan     chan struct{}
	stopOnce     sync.Once
	running      bool
	runningMutex sync.RWMutex
}

// NewSportsClient creates a public Sports Channel client.
func NewSportsClient(reconnectDelay time.Duration) SportsClient {
	return newSportsClient(wsSportsURL, reconnectDelay)
}

func newSportsClient(url string, reconnectDelay time.Duration) *sportsWebSocketClient {
	return &sportsWebSocketClient{
		url:            url,
		reconnectDelay: reconnectDelay,
		stopChan:       make(chan struct{}),
	}
}

func (s *sportsWebSocketClient) SetOnSportsUpdate(callback func(result *SportResult)) {
	s.callbackMutex.Lock()
	s.onSportsUpdate = callback
	s.callbackMutex.Unlock()
}

func (s *sportsWebSocketClient) Start() error {
	s.runningMutex.Lock()
	if s.running {
		s.runningMutex.Unlock()
		return fmt.Errorf("Sports WebSocket client already running")
	}
	s.stopChan = make(chan struct{})
	s.stopOnce = sync.Once{}
	s.running = true
	stop := s.stopChan
	s.runningMutex.Unlock()

	conn, err := s.connect()
	if err != nil {
		s.runningMutex.Lock()
		s.running = false
		s.runningMutex.Unlock()
		return err
	}
	if channelClosed(stop) {
		s.clearConn(conn)
		_ = conn.Close()
		return fmt.Errorf("Sports WebSocket client stopped during Start")
	}

	go s.run(conn, stop)
	return nil
}

func (s *sportsWebSocketClient) Stop() {
	s.runningMutex.Lock()
	if !s.running {
		s.runningMutex.Unlock()
		return
	}
	s.running = false
	stop := s.stopChan
	s.runningMutex.Unlock()

	s.stopOnce.Do(func() { close(stop) })
	if conn := s.currentConn(); conn != nil {
		_ = conn.Close()
	}
}

func (s *sportsWebSocketClient) IsRunning() bool {
	s.runningMutex.RLock()
	defer s.runningMutex.RUnlock()
	return s.running
}

func (s *sportsWebSocketClient) run(conn *websocket.Conn, stop <-chan struct{}) {
	for {
		_ = s.listen(conn, stop)
		s.clearConn(conn)
		_ = conn.Close()

		if channelClosed(stop) {
			return
		}
		for {
			if !waitForReconnect(stop, s.reconnectDelay) {
				return
			}
			nextConn, err := s.connect()
			if err != nil {
				continue
			}
			if channelClosed(stop) {
				_ = nextConn.Close()
				return
			}
			conn = nextConn
			break
		}
	}
}

func (s *sportsWebSocketClient) connect() (*websocket.Conn, error) {
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

	conn, _, err := dialer.Dial(s.url, nil)
	if err != nil {
		return nil, fmt.Errorf("connect Sports Channel: %w", err)
	}
	s.connMutex.Lock()
	s.conn = conn
	s.connMutex.Unlock()
	return conn, nil
}

func (s *sportsWebSocketClient) listen(conn *websocket.Conn, stop <-chan struct{}) error {
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		if messageType != websocket.TextMessage {
			continue
		}
		if string(message) == "ping" {
			if err := s.writePong(conn); err != nil {
				return err
			}
			continue
		}
		if channelClosed(stop) {
			return nil
		}

		var result SportResult
		if err := json.Unmarshal(message, &result); err != nil || result.Slug == "" {
			continue
		}
		s.callbackMutex.RLock()
		callback := s.onSportsUpdate
		s.callbackMutex.RUnlock()
		if callback != nil {
			callback(&result)
		}
	}
}

func (s *sportsWebSocketClient) writePong(conn *websocket.Conn) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()
	if err := conn.WriteMessage(websocket.TextMessage, []byte("pong")); err != nil {
		return fmt.Errorf("send Sports Channel pong: %w", err)
	}
	return nil
}

func (s *sportsWebSocketClient) currentConn() *websocket.Conn {
	s.connMutex.RLock()
	defer s.connMutex.RUnlock()
	return s.conn
}

func (s *sportsWebSocketClient) clearConn(conn *websocket.Conn) {
	s.connMutex.Lock()
	if s.conn == conn {
		s.conn = nil
	}
	s.connMutex.Unlock()
}

func channelClosed(channel <-chan struct{}) bool {
	select {
	case <-channel:
		return true
	default:
		return false
	}
}

func waitForReconnect(stop <-chan struct{}, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-stop:
		return false
	case <-timer.C:
		return true
	}
}
