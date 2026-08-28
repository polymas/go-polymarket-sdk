package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/polymas/go-polymarket-sdk/types"
)

const testMarketConditionID = "0x0000000000000000000000000000000000000000000000000000000000000001"

func TestMarketClientProtocolAndEvents(t *testing.T) {
	t.Parallel()

	requests := make(chan marketSubscriptionRequest, 1)
	heartbeats := make(chan string, 1)
	serverErrors := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		var request marketSubscriptionRequest
		if err := conn.ReadJSON(&request); err != nil {
			serverErrors <- "read initial subscription: " + err.Error()
			return
		}
		requests <- request

		messageType, message, err := conn.ReadMessage()
		if err != nil {
			serverErrors <- "read heartbeat: " + err.Error()
			return
		}
		if messageType != websocket.TextMessage {
			serverErrors <- "heartbeat was not a text message"
			return
		}
		heartbeats <- string(message)
		if err := conn.WriteMessage(websocket.TextMessage, []byte("PONG")); err != nil {
			return
		}

		events := `[
			{"event_type":"book","asset_id":"1","market":"` + testMarketConditionID + `","bids":[{"price":"0.48","size":"30"},{"price":"0.49","size":"20"}],"asks":[{"price":"0.52","size":"25"},{"price":"0.51","size":"10"}],"timestamp":"1757908892351","hash":"book-hash"},
			{"event_type":"price_change","market":"` + testMarketConditionID + `","price_changes":[{"asset_id":"1","price":"0.5","size":"200","side":"BUY","hash":"change-hash","best_bid":"0.5","best_ask":"1"}],"timestamp":"1757908892352"},
			{"event_type":"last_trade_price","asset_id":"1","market":"` + testMarketConditionID + `","price":"0.456","size":"219.217767","side":"BUY","timestamp":"1757908892353","transaction_hash":"0xtrade"},
			{"event_type":"tick_size_change","asset_id":"1","market":"` + testMarketConditionID + `","old_tick_size":"0.01","new_tick_size":"0.001","timestamp":"1757908892354"},
			{"event_type":"best_bid_ask","market":"` + testMarketConditionID + `","asset_id":"1","best_bid":"0.73","best_ask":"0.77","spread":"0.04","timestamp":"1757908892355"},
			{"event_type":"new_market","id":"1031769","question":"Will it happen?","market":"` + testMarketConditionID + `","slug":"will-it-happen","description":"Rules","assets_ids":["1","2"],"outcomes":["Yes","No"],"event_message":{"id":"125819","ticker":"ticker","slug":"event-slug","title":"Event","description":"Event rules"},"timestamp":"1757908892356","tags":["test"],"condition_id":"` + testMarketConditionID + `","active":true,"clob_token_ids":["1","2"],"sports_market_type":"","line":"","game_start_time":"","order_price_min_tick_size":"0.01","group_item_title":"Group"},
			{"event_type":"market_resolved","id":"1031769","market":"` + testMarketConditionID + `","assets_ids":["1","2"],"winning_asset_id":"1","winning_outcome":"Yes","timestamp":"1757908892357","tags":["test"]}
		]`
		if err := conn.WriteMessage(websocket.TextMessage, []byte(events)); err != nil {
			return
		}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	client, err := newMarketClient(
		strings.Replace(server.URL, "http://", "ws://", 1),
		10*time.Millisecond,
		20*time.Millisecond,
		MarketSubscriptionOptions{
			InitialDump:          false,
			Level:                MarketSubscriptionLevel3,
			CustomFeatureEnabled: true,
		},
	)
	if err != nil {
		t.Fatalf("newMarketClient() error = %v", err)
	}
	defer client.Stop()

	marketEvents := make(chan MarketEvent, 7)
	bookUpdates := make(chan struct {
		assetID string
		book    *types.BookSnapshot
	}, 1)
	client.SetOnMarketEvent(func(event MarketEvent) { marketEvents <- event })
	client.SetOnBookUpdate(func(assetID string, book *types.BookSnapshot) {
		bookUpdates <- struct {
			assetID string
			book    *types.BookSnapshot
		}{assetID: assetID, book: book}
	})
	if err := client.Start([]string{"2", "1", "1"}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	select {
	case request := <-requests:
		if request.Type != "market" || !reflect.DeepEqual(request.AssetIDs, []string{"1", "2"}) ||
			request.InitialDump || request.Level != MarketSubscriptionLevel3 ||
			!request.CustomFeatureEnabled {
			t.Fatalf("initial subscription = %#v", request)
		}
	case message := <-serverErrors:
		t.Fatal(message)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initial subscription")
	}
	select {
	case heartbeat := <-heartbeats:
		if heartbeat != "PING" {
			t.Fatalf("heartbeat = %q, want plain text PING", heartbeat)
		}
	case message := <-serverErrors:
		t.Fatal(message)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for heartbeat")
	}

	received := make(map[MarketEventType]MarketEvent)
	for len(received) < 7 {
		select {
		case event := <-marketEvents:
			received[event.Type()] = event
		case message := <-serverErrors:
			t.Fatal(message)
		case <-time.After(2 * time.Second):
			t.Fatalf("received %d Market event types, want 7", len(received))
		}
	}

	tick := received[MarketEventTickSizeChange].(*MarketTickSizeChangeEvent)
	if tick.TokenID.String() != "1" || tick.OldTickSize != types.TickSize0_01 ||
		tick.NewTickSize != types.TickSize0_001 {
		t.Fatalf("tick size event = %#v", tick)
	}
	priceChange := received[MarketEventPriceChange].(*MarketPriceChangeEvent)
	if len(priceChange.PriceChanges) != 1 || priceChange.PriceChanges[0].Side != types.OrderSideBUY ||
		priceChange.PriceChanges[0].BestBid != "0.5" {
		t.Fatalf("price change event = %#v", priceChange)
	}
	newMarket := received[MarketEventNewMarket].(*MarketNewMarketEvent)
	if newMarket.ConditionID.String() != testMarketConditionID || len(newMarket.ClobTokenIDs) != 2 ||
		newMarket.OrderPriceMinTickSize != types.TickSize0_01 {
		t.Fatalf("new market event = %#v", newMarket)
	}
	resolved := received[MarketEventMarketResolved].(*MarketResolvedEvent)
	if resolved.WinningTokenID.String() != "1" || resolved.WinningOutcome != "Yes" {
		t.Fatalf("resolved event = %#v", resolved)
	}

	select {
	case update := <-bookUpdates:
		if update.assetID != "1" || update.book.BestBid == nil || update.book.BestAsk == nil ||
			update.book.BestBid.Price != 0.49 || update.book.BestBid.Size != 20 ||
			update.book.BestAsk.Price != 0.51 || update.book.BestAsk.Size != 10 ||
			update.book.TS.UnixMilli() != 1757908892351 {
			t.Fatalf("legacy book update = asset %q, book %#v", update.assetID, update.book)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for legacy book callback")
	}
	if err := client.Start([]string{"1"}); err == nil {
		t.Fatal("duplicate Start() succeeded")
	}
}

func TestMarketClientDynamicSubscriptionsAndReconnect(t *testing.T) {
	t.Parallel()

	var connections atomic.Int32
	initialRequests := make(chan marketSubscriptionRequest, 2)
	updates := make(chan marketSubscriptionUpdate, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		connectionNumber := connections.Add(1)

		var initial marketSubscriptionRequest
		if err := conn.ReadJSON(&initial); err != nil {
			return
		}
		initialRequests <- initial
		updatesOnConnection := 0
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if string(message) == "PING" {
				_ = conn.WriteMessage(websocket.TextMessage, []byte("PONG"))
				continue
			}
			var update marketSubscriptionUpdate
			if err := json.Unmarshal(message, &update); err != nil {
				return
			}
			updates <- update
			updatesOnConnection++
			if connectionNumber == 1 && updatesOnConnection == 2 {
				return
			}
		}
	}))
	defer server.Close()

	client, err := newMarketClient(
		strings.Replace(server.URL, "http://", "ws://", 1),
		10*time.Millisecond,
		time.Hour,
		MarketSubscriptionOptions{
			InitialDump:          true,
			Level:                MarketSubscriptionLevel2,
			CustomFeatureEnabled: true,
		},
	)
	if err != nil {
		t.Fatalf("newMarketClient() error = %v", err)
	}
	defer client.Stop()
	if err := client.Start([]string{"1", "2"}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	assertInitialMarketRequest(t, initialRequests, []string{"1", "2"})

	if err := client.UpdateSubscription([]string{"2", "3"}); err != nil {
		t.Fatalf("UpdateSubscription() error = %v", err)
	}
	assertMarketUpdate(t, updates, "unsubscribe", []string{"1"})
	assertMarketUpdate(t, updates, "subscribe", []string{"3"})
	assertInitialMarketRequest(t, initialRequests, []string{"2", "3"})

	if err := client.SubscribeTokens([]string{"4", "4"}); err != nil {
		t.Fatalf("SubscribeTokens() error = %v", err)
	}
	assertMarketUpdate(t, updates, "subscribe", []string{"4"})
	if err := client.UnsubscribeTokens([]string{"2"}); err != nil {
		t.Fatalf("UnsubscribeTokens() error = %v", err)
	}
	assertMarketUpdate(t, updates, "unsubscribe", []string{"2"})
	if connections.Load() < 2 {
		t.Fatalf("connections = %d, want at least 2", connections.Load())
	}
}

func TestMarketClientOptionsAndRestart(t *testing.T) {
	t.Parallel()

	if _, err := NewClientWithOptions(time.Second, MarketSubscriptionOptions{}); err == nil {
		t.Fatal("NewClientWithOptions accepted level 0")
	}
	if _, err := NewClientWithOptions(time.Second, MarketSubscriptionOptions{Level: 4}); err == nil {
		t.Fatal("NewClientWithOptions accepted level 4")
	}

	requests := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var request marketSubscriptionRequest
		if err := conn.ReadJSON(&request); err != nil {
			return
		}
		requests <- struct{}{}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	client, err := newMarketClient(
		strings.Replace(server.URL, "http://", "ws://", 1),
		10*time.Millisecond,
		time.Hour,
		DefaultMarketSubscriptionOptions(),
	)
	if err != nil {
		t.Fatalf("newMarketClient() error = %v", err)
	}
	for run := 0; run < 2; run++ {
		if err := client.Start([]string{"1"}); err != nil {
			t.Fatalf("Start() run %d error = %v", run+1, err)
		}
		select {
		case <-requests:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for subscription on run %d", run+1)
		}
		client.Stop()
		if client.IsRunning() {
			t.Fatalf("client is running after Stop() on run %d", run+1)
		}
	}
}

func assertInitialMarketRequest(
	t *testing.T,
	requests <-chan marketSubscriptionRequest,
	wantAssets []string,
) {
	t.Helper()
	select {
	case request := <-requests:
		if request.Type != "market" || !reflect.DeepEqual(request.AssetIDs, wantAssets) ||
			!request.InitialDump || request.Level != MarketSubscriptionLevel2 ||
			!request.CustomFeatureEnabled {
			t.Fatalf("initial subscription = %#v, want assets %v", request, wantAssets)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for initial subscription with assets %v", wantAssets)
	}
}

func assertMarketUpdate(
	t *testing.T,
	updates <-chan marketSubscriptionUpdate,
	wantOperation string,
	wantAssets []string,
) {
	t.Helper()
	select {
	case update := <-updates:
		if update.Operation != wantOperation || !reflect.DeepEqual(update.AssetIDs, wantAssets) ||
			update.Level != MarketSubscriptionLevel2 || !update.CustomFeatureEnabled {
			t.Fatalf("subscription update = %#v, want operation %q assets %v", update, wantOperation, wantAssets)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s update with assets %v", wantOperation, wantAssets)
	}
}
