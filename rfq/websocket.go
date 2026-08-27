package rfq

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type WSOption func(*WebSocketClient)

func WithWebSocketURL(url string) WSOption {
	return func(c *WebSocketClient) { c.url = url }
}

type WebSocketClient struct {
	url            string
	config         Config
	reconnectDelay time.Duration

	stateMu sync.RWMutex
	writeMu sync.Mutex
	conn    *websocket.Conn
	running bool
	stop    chan struct{}
	done    chan struct{}
	onEvent func(Event)
}

func NewWebSocketClient(config Config, reconnectDelay time.Duration, options ...WSOption) *WebSocketClient {
	c := &WebSocketClient{url: WSURL, config: config, reconnectDelay: reconnectDelay}
	for _, option := range options {
		option(c)
	}
	return c
}

func (c *WebSocketClient) SetOnEvent(callback func(Event)) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.onEvent = callback
}

func (c *WebSocketClient) Start() error {
	if c.config.Credentials == nil {
		return fmt.Errorf("RFQ WebSocket requires CLOB API credentials")
	}
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.running {
		return fmt.Errorf("RFQ WebSocket already running")
	}
	c.stop, c.done, c.running = make(chan struct{}), make(chan struct{}), true
	go c.run(c.stop, c.done)
	return nil
}

func (c *WebSocketClient) Stop() {
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

func (c *WebSocketClient) IsRunning() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.running
}

func (c *WebSocketClient) SendQuote(quote WSQuote) error {
	quote.Type = "RFQ_QUOTE"
	return c.writeJSON(quote)
}

func (c *WebSocketClient) CancelQuote(rfqID, quoteID string) error {
	return c.writeJSON(struct {
		Type          string `json:"type"`
		RFQID         string `json:"rfq_id"`
		QuoteID       string `json:"quote_id"`
		SignerAddress string `json:"signer_address"`
		MakerAddress  string `json:"maker_address"`
	}{"RFQ_QUOTE_CANCEL", rfqID, quoteID, c.config.Identity.SignerAddress, c.config.Identity.MakerAddress})
}

func (c *WebSocketClient) Confirm(rfqID, quoteID string, decision ConfirmationDecision) error {
	return c.writeJSON(struct {
		Type     string               `json:"type"`
		RFQID    string               `json:"rfq_id"`
		QuoteID  string               `json:"quote_id"`
		Decision ConfirmationDecision `json:"decision"`
	}{"RFQ_CONFIRMATION_RESPONSE", rfqID, quoteID, decision})
}

func (c *WebSocketClient) run(stop <-chan struct{}, done chan<- struct{}) {
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

func (c *WebSocketClient) connectAndRead(stop <-chan struct{}) error {
	conn, _, err := websocket.DefaultDialer.Dial(c.url, nil)
	if err != nil {
		return err
	}
	c.stateMu.Lock()
	c.conn = conn
	c.stateMu.Unlock()
	defer func() {
		_ = conn.Close()
		c.stateMu.Lock()
		if c.conn == conn {
			c.conn = nil
		}
		c.stateMu.Unlock()
	}()

	auth := struct {
		Type     string   `json:"type"`
		Auth     any      `json:"auth"`
		Identity Identity `json:"identity"`
	}{Type: "auth", Auth: c.config.Credentials, Identity: c.config.Identity}
	if err := c.writeJSONTo(conn, auth); err != nil {
		return err
	}

	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(payload, &envelope); err != nil {
			continue
		}
		c.stateMu.RLock()
		callback := c.onEvent
		c.stateMu.RUnlock()
		if callback != nil {
			callback(Event{Type: envelope.Type, Payload: append(json.RawMessage(nil), payload...)})
		}
		select {
		case <-stop:
			return nil
		default:
		}
	}
}

func (c *WebSocketClient) writeJSON(value any) error {
	c.stateMu.RLock()
	conn := c.conn
	running := c.running
	c.stateMu.RUnlock()
	if !running || conn == nil {
		return fmt.Errorf("RFQ WebSocket is not connected")
	}
	return c.writeJSONTo(conn, value)
}

func (c *WebSocketClient) writeJSONTo(conn *websocket.Conn, value any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return conn.WriteJSON(value)
}
