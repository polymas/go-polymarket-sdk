package websocket

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

const (
	wsUserURL             = "wss://ws-subscriptions-clob.polymarket.com/ws/user"
	userHeartbeatInterval = 10 * time.Second
)

// UserClient receives authenticated order and trade events. Market filters are
// condition IDs; an empty slice passed to Start subscribes to all user markets.
type UserClient interface {
	SetOnOrderUpdate(callback func(event *UserOrderEvent))
	SetOnTradeUpdate(callback func(event *UserTradeEvent))
	Start(markets []types.ConditionID) error
	SubscribeMarkets(markets []types.ConditionID) error
	UnsubscribeMarkets(markets []types.ConditionID) error
	Stop()
	IsRunning() bool
}

type userWebSocketClient struct {
	url               string
	creds             types.ApiCreds
	reconnectDelay    time.Duration
	heartbeatInterval time.Duration

	conn       *websocket.Conn
	connMutex  sync.RWMutex
	writeMutex sync.Mutex

	markets      map[types.ConditionID]struct{}
	allMarkets   bool
	marketsMutex sync.RWMutex

	onOrderUpdate func(event *UserOrderEvent)
	onTradeUpdate func(event *UserTradeEvent)
	callbackMutex sync.RWMutex

	stopChan     chan struct{}
	stopOnce     sync.Once
	running      bool
	runningMutex sync.RWMutex
}

type userSubscriptionRequest struct {
	Auth    types.ApiCreds `json:"auth"`
	Type    string         `json:"type"`
	Markets *[]string      `json:"markets,omitempty"`
}

type userSubscriptionUpdate struct {
	Operation string   `json:"operation"`
	Markets   []string `json:"markets"`
}

// NewUserClient creates an authenticated User Channel client. Credentials are
// the CLOB API credentials returned by clob.Client.GetAPICreds, not an L1
// wallet signature.
func NewUserClient(creds types.ApiCreds, reconnectDelay time.Duration) UserClient {
	return newUserClient(wsUserURL, creds, reconnectDelay, userHeartbeatInterval)
}

func newUserClient(
	url string,
	creds types.ApiCreds,
	reconnectDelay time.Duration,
	heartbeatInterval time.Duration,
) *userWebSocketClient {
	return &userWebSocketClient{
		url:               url,
		creds:             creds,
		reconnectDelay:    reconnectDelay,
		heartbeatInterval: heartbeatInterval,
		markets:           make(map[types.ConditionID]struct{}),
		stopChan:          make(chan struct{}),
	}
}

func (u *userWebSocketClient) SetOnOrderUpdate(callback func(event *UserOrderEvent)) {
	u.callbackMutex.Lock()
	u.onOrderUpdate = callback
	u.callbackMutex.Unlock()
}

func (u *userWebSocketClient) SetOnTradeUpdate(callback func(event *UserTradeEvent)) {
	u.callbackMutex.Lock()
	u.onTradeUpdate = callback
	u.callbackMutex.Unlock()
}

func (u *userWebSocketClient) Start(markets []types.ConditionID) error {
	if u.creds.Key == "" || u.creds.Secret == "" || u.creds.Passphrase == "" {
		return fmt.Errorf("complete CLOB API credentials are required for User Channel")
	}

	u.runningMutex.Lock()
	if u.running {
		u.runningMutex.Unlock()
		return fmt.Errorf("User WebSocket client already running")
	}
	u.stopChan = make(chan struct{})
	u.stopOnce = sync.Once{}
	u.running = true
	u.runningMutex.Unlock()

	u.marketsMutex.Lock()
	u.markets = conditionIDSet(markets)
	u.allMarkets = len(markets) == 0
	u.marketsMutex.Unlock()

	conn, err := u.connect()
	if err != nil {
		u.runningMutex.Lock()
		u.running = false
		u.runningMutex.Unlock()
		return err
	}

	go u.run(conn)
	return nil
}

func (u *userWebSocketClient) SubscribeMarkets(markets []types.ConditionID) error {
	return u.updateMarkets("subscribe", markets)
}

func (u *userWebSocketClient) UnsubscribeMarkets(markets []types.ConditionID) error {
	return u.updateMarkets("unsubscribe", markets)
}

func (u *userWebSocketClient) updateMarkets(operation string, markets []types.ConditionID) error {
	if len(markets) == 0 {
		return fmt.Errorf("at least one condition ID is required")
	}
	if !u.IsRunning() {
		return fmt.Errorf("User WebSocket client is not running")
	}

	u.marketsMutex.RLock()
	allMarkets := u.allMarkets
	u.marketsMutex.RUnlock()
	if allMarkets {
		return fmt.Errorf("dynamic market updates require Start with explicit condition IDs")
	}

	u.writeMutex.Lock()
	defer u.writeMutex.Unlock()

	conn := u.currentConn()
	if conn == nil {
		return fmt.Errorf("User WebSocket is not connected")
	}

	marketStrings := conditionIDStrings(markets)
	if err := conn.WriteJSON(userSubscriptionUpdate{
		Operation: operation,
		Markets:   marketStrings,
	}); err != nil {
		return fmt.Errorf("send User Channel %s: %w", operation, err)
	}

	u.marketsMutex.Lock()
	for _, market := range markets {
		if operation == "subscribe" {
			u.markets[market] = struct{}{}
		} else {
			delete(u.markets, market)
		}
	}
	u.marketsMutex.Unlock()
	return nil
}

func (u *userWebSocketClient) Stop() {
	u.runningMutex.Lock()
	if !u.running {
		u.runningMutex.Unlock()
		return
	}
	u.running = false
	u.runningMutex.Unlock()

	u.stopOnce.Do(func() { close(u.stopChan) })
	if conn := u.currentConn(); conn != nil {
		_ = conn.Close()
	}
}

func (u *userWebSocketClient) IsRunning() bool {
	u.runningMutex.RLock()
	defer u.runningMutex.RUnlock()
	return u.running
}

func (u *userWebSocketClient) run(conn *websocket.Conn) {
	for {
		_ = u.listen(conn)
		u.clearConn(conn)
		_ = conn.Close()

		if u.stopped() {
			return
		}
		for {
			if !u.waitReconnect() {
				return
			}
			nextConn, err := u.connect()
			if err != nil {
				continue
			}
			conn = nextConn
			break
		}
	}
}

func (u *userWebSocketClient) connect() (*websocket.Conn, error) {
	u.writeMutex.Lock()
	defer u.writeMutex.Unlock()

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

	conn, _, err := dialer.Dial(u.url, nil)
	if err != nil {
		return nil, fmt.Errorf("connect User Channel: %w", err)
	}

	request := userSubscriptionRequest{
		Auth: u.creds,
		Type: "user",
	}
	u.marketsMutex.RLock()
	if !u.allMarkets {
		markets := conditionIDSetStrings(u.markets)
		request.Markets = &markets
	}
	u.marketsMutex.RUnlock()

	if err := conn.WriteJSON(request); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("send User Channel subscription: %w", err)
	}
	if u.stopped() {
		_ = conn.Close()
		return nil, fmt.Errorf("User WebSocket client stopped")
	}

	u.connMutex.Lock()
	u.conn = conn
	u.connMutex.Unlock()
	return conn, nil
}

func (u *userWebSocketClient) listen(conn *websocket.Conn) error {
	heartbeatStop := make(chan struct{})
	go u.heartbeat(conn, heartbeatStop)
	defer close(heartbeatStop)

	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		if messageType != websocket.TextMessage || string(message) == "PONG" {
			continue
		}
		u.dispatch(message)
	}
}

func (u *userWebSocketClient) heartbeat(conn *websocket.Conn, stop <-chan struct{}) {
	ticker := time.NewTicker(u.heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-u.stopChan:
			return
		case <-ticker.C:
			u.writeMutex.Lock()
			err := conn.WriteMessage(websocket.TextMessage, []byte("PING"))
			u.writeMutex.Unlock()
			if err != nil {
				_ = conn.Close()
				return
			}
		}
	}
}

func (u *userWebSocketClient) dispatch(message []byte) {
	var raw json.RawMessage
	if len(message) > 0 && message[0] == '[' {
		var messages []json.RawMessage
		if err := json.Unmarshal(message, &messages); err != nil {
			return
		}
		for _, item := range messages {
			u.dispatch(item)
		}
		return
	}
	raw = message

	var envelope struct {
		EventType string `json:"event_type"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return
	}

	switch envelope.EventType {
	case "order":
		var event UserOrderEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			return
		}
		u.callbackMutex.RLock()
		callback := u.onOrderUpdate
		u.callbackMutex.RUnlock()
		if callback != nil {
			callback(&event)
		}
	case "trade":
		var event UserTradeEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			return
		}
		u.callbackMutex.RLock()
		callback := u.onTradeUpdate
		u.callbackMutex.RUnlock()
		if callback != nil {
			callback(&event)
		}
	}
}

func (u *userWebSocketClient) currentConn() *websocket.Conn {
	u.connMutex.RLock()
	defer u.connMutex.RUnlock()
	return u.conn
}

func (u *userWebSocketClient) clearConn(conn *websocket.Conn) {
	u.connMutex.Lock()
	if u.conn == conn {
		u.conn = nil
	}
	u.connMutex.Unlock()
}

func (u *userWebSocketClient) stopped() bool {
	select {
	case <-u.stopChan:
		return true
	default:
		return false
	}
}

func (u *userWebSocketClient) waitReconnect() bool {
	timer := time.NewTimer(u.reconnectDelay)
	defer timer.Stop()
	select {
	case <-u.stopChan:
		return false
	case <-timer.C:
		return true
	}
}

func conditionIDSet(markets []types.ConditionID) map[types.ConditionID]struct{} {
	set := make(map[types.ConditionID]struct{}, len(markets))
	for _, market := range markets {
		set[market] = struct{}{}
	}
	return set
}

func conditionIDStrings(markets []types.ConditionID) []string {
	result := make([]string, 0, len(markets))
	seen := make(map[types.ConditionID]struct{}, len(markets))
	for _, market := range markets {
		if _, exists := seen[market]; exists {
			continue
		}
		seen[market] = struct{}{}
		result = append(result, market.String())
	}
	sort.Strings(result)
	return result
}

func conditionIDSetStrings(markets map[types.ConditionID]struct{}) []string {
	result := make([]string, 0, len(markets))
	for market := range markets {
		result = append(result, market.String())
	}
	sort.Strings(result)
	return result
}
