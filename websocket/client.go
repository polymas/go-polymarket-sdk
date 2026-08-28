package websocket

import (
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/polymas/go-polymarket-sdk/types"
)

const (
	wsMarketURL             = "wss://ws-subscriptions-clob.polymarket.com/ws/market"
	marketHeartbeatInterval = 10 * time.Second
)

// Client is the public Market Channel client. SetOnBookUpdate is retained for
// callers that only need the best bid/ask view; SetOnMarketEvent exposes the
// complete typed protocol.
type Client interface {
	SetOnBookUpdate(callback func(tokenID string, snapshot *types.BookSnapshot))
	SetOnMarketEvent(callback func(event MarketEvent))
	Start(tokenIDs []string) error
	Stop()
	IsRunning() bool
	UpdateSubscription(tokenIDs []string) error
	SubscribeTokens(tokenIDs []string) error
	UnsubscribeTokens(tokenIDs []string) error
	// Deprecated: use SubscribeTokens.
	SubscribeAssets(tokenIDs []string) error
	// Deprecated: use UnsubscribeTokens.
	UnsubscribeAssets(tokenIDs []string) error
}

// MarketSubscriptionLevel controls the depth/detail level requested from the
// Market Channel.
type MarketSubscriptionLevel int

const (
	MarketSubscriptionLevel1 MarketSubscriptionLevel = 1
	MarketSubscriptionLevel2 MarketSubscriptionLevel = 2
	MarketSubscriptionLevel3 MarketSubscriptionLevel = 3
)

// MarketSubscriptionOptions controls the initial subscription and is restored
// after reconnects. CustomFeatureEnabled enables best_bid_ask, new_market, and
// market_resolved events.
type MarketSubscriptionOptions struct {
	InitialDump          bool
	Level                MarketSubscriptionLevel
	CustomFeatureEnabled bool
}

// DefaultMarketSubscriptionOptions returns the official protocol defaults.
func DefaultMarketSubscriptionOptions() MarketSubscriptionOptions {
	return MarketSubscriptionOptions{
		InitialDump: true,
		Level:       MarketSubscriptionLevel2,
	}
}

func (o MarketSubscriptionOptions) validate() error {
	if o.Level < MarketSubscriptionLevel1 || o.Level > MarketSubscriptionLevel3 {
		return fmt.Errorf("Market subscription level must be 1, 2, or 3")
	}
	return nil
}

type webSocketClient struct {
	url               string
	reconnectDelay    time.Duration
	heartbeatInterval time.Duration
	options           MarketSubscriptionOptions

	conn       *websocket.Conn
	connMutex  sync.RWMutex
	writeMutex sync.Mutex

	subscribedIDs   map[string]struct{}
	subscribedMutex sync.RWMutex

	onBookUpdate  func(tokenID string, snapshot *types.BookSnapshot)
	onMarketEvent func(event MarketEvent)
	callbackMutex sync.RWMutex

	stopChan     chan struct{}
	stopOnce     sync.Once
	running      bool
	runningMutex sync.RWMutex
}

// NewClient creates a Market Channel client using the official defaults:
// initial dump enabled, level 2, and custom features disabled.
func NewClient(reconnectDelay time.Duration) Client {
	client, _ := newMarketClient(
		wsMarketURL,
		reconnectDelay,
		marketHeartbeatInterval,
		DefaultMarketSubscriptionOptions(),
	)
	return client
}

// NewClientWithOptions creates a configurable Market Channel client. Callers
// should start with DefaultMarketSubscriptionOptions and change only the
// fields they need.
func NewClientWithOptions(
	reconnectDelay time.Duration,
	options MarketSubscriptionOptions,
) (Client, error) {
	return newMarketClient(wsMarketURL, reconnectDelay, marketHeartbeatInterval, options)
}

func newMarketClient(
	url string,
	reconnectDelay time.Duration,
	heartbeatInterval time.Duration,
	options MarketSubscriptionOptions,
) (*webSocketClient, error) {
	if err := options.validate(); err != nil {
		return nil, err
	}
	return &webSocketClient{
		url:               url,
		reconnectDelay:    reconnectDelay,
		heartbeatInterval: heartbeatInterval,
		options:           options,
		subscribedIDs:     make(map[string]struct{}),
		stopChan:          make(chan struct{}),
	}, nil
}

func (w *webSocketClient) SetOnBookUpdate(callback func(tokenID string, snapshot *types.BookSnapshot)) {
	w.callbackMutex.Lock()
	w.onBookUpdate = callback
	w.callbackMutex.Unlock()
}

func (w *webSocketClient) SetOnMarketEvent(callback func(event MarketEvent)) {
	w.callbackMutex.Lock()
	w.onMarketEvent = callback
	w.callbackMutex.Unlock()
}
