package rtds

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/polymas/go-polymarket-sdk/types"
)

const URL = "wss://ws-live-data.polymarket.com"

type TWAPWindow int

const (
	TWAP30Seconds TWAPWindow = 30
	TWAP60Seconds TWAPWindow = 60
)

type Subscription struct {
	Window TWAPWindow
	Symbol string // lowercase slash-delimited; empty subscribes to every symbol
}

type Option func(*Client)

func WithURL(url string) Option { return func(c *Client) { c.url = url } }
func WithHeartbeatInterval(interval time.Duration) Option {
	return func(c *Client) { c.heartbeatInterval = interval }
}

type Client struct {
	url               string
	reconnectDelay    time.Duration
	heartbeatInterval time.Duration

	stateMu       sync.RWMutex
	writeMu       sync.Mutex
	conn          *websocket.Conn
	running       bool
	stop          chan struct{}
	done          chan struct{}
	onTWAPUpdate  func(*types.ChainlinkTWAPEvent)
	subscriptions map[Subscription]struct{}
}

func NewClient(reconnectDelay time.Duration, options ...Option) *Client {
	c := &Client{url: URL, reconnectDelay: reconnectDelay, heartbeatInterval: 5 * time.Second, subscriptions: make(map[Subscription]struct{})}
	for _, option := range options {
		option(c)
	}
	return c
}

func (c *Client) SetOnTWAPUpdate(callback func(*types.ChainlinkTWAPEvent)) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.onTWAPUpdate = callback
}

func (c *Client) Start() error {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.running {
		return fmt.Errorf("RTDS client already running")
	}
	c.stop, c.done, c.running = make(chan struct{}), make(chan struct{}), true
	go c.run(c.stop, c.done)
	return nil
}

func (c *Client) Stop() {
	c.stateMu.Lock()
	if !c.running {
		c.stateMu.Unlock()
		return
	}
	stop, done, conn := c.stop, c.done, c.conn
	c.running = false
	close(stop)
	c.stateMu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
	<-done
}

func (c *Client) IsRunning() bool { c.stateMu.RLock(); defer c.stateMu.RUnlock(); return c.running }

func (c *Client) Subscribe(subscriptions []Subscription) error {
	if err := validateSubscriptions(subscriptions); err != nil {
		return err
	}
	c.stateMu.Lock()
	for _, subscription := range subscriptions {
		c.subscriptions[normalize(subscription)] = struct{}{}
	}
	conn := c.conn
	c.stateMu.Unlock()
	if conn == nil {
		return nil
	}
	return c.send(conn, "subscribe", subscriptions)
}

func (c *Client) Unsubscribe(subscriptions []Subscription) error {
	if err := validateSubscriptions(subscriptions); err != nil {
		return err
	}
	c.stateMu.Lock()
	for _, subscription := range subscriptions {
		delete(c.subscriptions, normalize(subscription))
	}
	conn := c.conn
	c.stateMu.Unlock()
	if conn == nil {
		return nil
	}
	return c.send(conn, "unsubscribe", subscriptions)
}

func (c *Client) run(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	for {
		if err := c.connectAndRead(stop); err == nil {
			return
		}
		select {
		case <-stop:
			return
		case <-time.After(c.reconnectDelay):
		}
	}
}

func (c *Client) connectAndRead(stop <-chan struct{}) error {
	conn, _, err := websocket.DefaultDialer.Dial(c.url, nil)
	if err != nil {
		return err
	}
	c.stateMu.Lock()
	c.conn = conn
	subscriptions := make([]Subscription, 0, len(c.subscriptions))
	for subscription := range c.subscriptions {
		subscriptions = append(subscriptions, subscription)
	}
	c.stateMu.Unlock()
	defer func() {
		_ = conn.Close()
		c.stateMu.Lock()
		if c.conn == conn {
			c.conn = nil
		}
		c.stateMu.Unlock()
	}()
	if len(subscriptions) > 0 {
		if err := c.send(conn, "subscribe", subscriptions); err != nil {
			return err
		}
	}

	heartbeatDone := make(chan struct{})
	defer close(heartbeatDone)
	go c.heartbeat(conn, heartbeatDone)
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var event types.ChainlinkTWAPEvent
		if err := json.Unmarshal(payload, &event); err != nil || event.Type != "update" {
			continue
		}
		c.stateMu.RLock()
		callback := c.onTWAPUpdate
		c.stateMu.RUnlock()
		if callback != nil {
			callback(&event)
		}
		select {
		case <-stop:
			return nil
		default:
		}
	}
}

func (c *Client) heartbeat(conn *websocket.Conn, done <-chan struct{}) {
	ticker := time.NewTicker(c.heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			c.writeMu.Lock()
			err := conn.WriteMessage(websocket.TextMessage, []byte("PING"))
			c.writeMu.Unlock()
			if err != nil {
				_ = conn.Close()
				return
			}
		}
	}
}

func (c *Client) send(conn *websocket.Conn, action string, subscriptions []Subscription) error {
	type wireSubscription struct {
		Topic   string `json:"topic"`
		Type    string `json:"type"`
		Filters string `json:"filters,omitempty"`
	}
	wire := make([]wireSubscription, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		subscription = normalize(subscription)
		item := wireSubscription{Topic: topic(subscription.Window), Type: "update"}
		if subscription.Symbol != "" {
			filter, _ := json.Marshal(struct {
				Symbol string `json:"symbol"`
			}{subscription.Symbol})
			item.Filters = string(filter)
		}
		wire = append(wire, item)
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return conn.WriteJSON(struct {
		Action        string             `json:"action"`
		Subscriptions []wireSubscription `json:"subscriptions"`
	}{action, wire})
}

func validateSubscriptions(subscriptions []Subscription) error {
	for _, subscription := range subscriptions {
		if subscription.Window != TWAP30Seconds && subscription.Window != TWAP60Seconds {
			return fmt.Errorf("TWAP window must be 30 or 60 seconds")
		}
		if subscription.Symbol != "" && (subscription.Symbol != strings.ToLower(subscription.Symbol) || !strings.Contains(subscription.Symbol, "/")) {
			return fmt.Errorf("symbol must be lowercase and slash-delimited")
		}
	}
	return nil
}

func normalize(subscription Subscription) Subscription {
	subscription.Symbol = strings.TrimSpace(subscription.Symbol)
	return subscription
}
func topic(window TWAPWindow) string {
	if window == TWAP30Seconds {
		return "crypto_prices_twap_thirty"
	}
	return "crypto_prices_twap_sixty"
}
