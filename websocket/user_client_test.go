package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/polymas/go-polymarket-sdk/types"
)

const (
	testConditionOne = "0x1111111111111111111111111111111111111111111111111111111111111111"
	testConditionTwo = "0x2222222222222222222222222222222222222222222222222222222222222222"
	testTokenOne     = "52114319501245915516055106046884209969926127482827954674443846427813813222426"
	testTokenTwo     = "60487116984468020978247225474488676749601001829886755968952521846780452448915"
)

var testUpgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

func TestUserClientProtocolEventsAndHeartbeat(t *testing.T) {
	t.Parallel()

	received := make(chan map[string]any, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		_, initial, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var initialMessage map[string]any
		if json.Unmarshal(initial, &initialMessage) == nil {
			received <- initialMessage
		}

		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{
			"event_type":"order","id":"0xorder","owner":"owner-key",
			"market":"`+testConditionOne+`","asset_id":"`+testTokenOne+`",
			"side":"SELL","order_owner":"owner-key","original_size":"10",
			"size_matched":"2.5","price":"0.57","associate_trades":["trade-1"],
			"outcome":"YES","type":"UPDATE","created_at":"1672290687",
			"expiration":"0","order_type":"GTC","status":"LIVE",
			"maker_address":"0xmaker","timestamp":"1672290687000"
		}`))
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{
			"event_type":"trade","type":"TRADE","id":"trade-1",
			"taker_order_id":"0xtaker","market":"`+testConditionOne+`",
			"asset_id":"`+testTokenOne+`","side":"BUY","size":"2.5",
			"price":"0.57","fee_rate_bps":"0","status":"MATCHED",
			"match_time":"1672290701","last_update":"1672290702","outcome":"YES",
			"owner":"owner-key","trade_owner":"trade-owner","maker_address":"0xmaker",
			"transaction_hash":"0xtx","bucket_index":3,
			"maker_orders":[{"order_id":"0xmaker-order","owner":"maker-owner",
			"maker_address":"0xmaker","matched_amount":"2.5","price":"0.57",
			"fee_rate_bps":"0","asset_id":"`+testTokenTwo+`","outcome":"NO","side":"SELL"}],
			"trader_side":"TAKER","timestamp":"1672290701000"
		}`))

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if string(message) == "PING" {
				received <- map[string]any{"heartbeat": string(message)}
				_ = conn.WriteMessage(websocket.TextMessage, []byte("PONG"))
				continue
			}
			var update map[string]any
			if json.Unmarshal(message, &update) == nil {
				received <- update
			}
		}
	}))
	defer server.Close()

	conditionOne := mustConditionID(t, testConditionOne)
	conditionTwo := mustConditionID(t, testConditionTwo)
	client := newUserClient(
		strings.Replace(server.URL, "http://", "ws://", 1),
		types.ApiCreds{Key: "api-key", Secret: "api-secret", Passphrase: "passphrase"},
		10*time.Millisecond,
		20*time.Millisecond,
	)
	defer client.Stop()

	orders := make(chan *UserOrderEvent, 1)
	trades := make(chan *UserTradeEvent, 1)
	client.SetOnOrderUpdate(func(event *UserOrderEvent) { orders <- event })
	client.SetOnTradeUpdate(func(event *UserTradeEvent) { trades <- event })

	if err := client.Start([]types.ConditionID{conditionOne}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := client.SubscribeMarkets([]types.ConditionID{conditionTwo}); err != nil {
		t.Fatalf("SubscribeMarkets() error = %v", err)
	}
	if err := client.UnsubscribeMarkets([]types.ConditionID{conditionOne}); err != nil {
		t.Fatalf("UnsubscribeMarkets() error = %v", err)
	}

	initial := receiveMap(t, received)
	if initial["type"] != "user" {
		t.Fatalf("initial type = %v, want user", initial["type"])
	}
	auth, ok := initial["auth"].(map[string]any)
	if !ok {
		t.Fatalf("initial auth = %#v", initial["auth"])
	}
	if auth["apiKey"] != "api-key" || auth["secret"] != "api-secret" || auth["passphrase"] != "passphrase" {
		t.Fatalf("initial auth = %#v", auth)
	}
	for _, wrongField := range []string{"address", "signature", "timestamp", "nonce"} {
		if _, exists := auth[wrongField]; exists {
			t.Fatalf("initial auth unexpectedly contains %q", wrongField)
		}
	}
	assertMarkets(t, initial, testConditionOne)

	updates := []map[string]any{receiveMap(t, received), receiveMap(t, received)}
	operations := map[string]string{}
	for _, update := range updates {
		operation, _ := update["operation"].(string)
		markets := update["markets"].([]any)
		operations[operation] = markets[0].(string)
	}
	if operations["subscribe"] != testConditionTwo || operations["unsubscribe"] != testConditionOne {
		t.Fatalf("dynamic operations = %#v", operations)
	}

	select {
	case event := <-orders:
		if event.ID != "0xorder" || event.Market != conditionOne || event.TokenID.String() != testTokenOne || event.SizeMatched != "2.5" || event.Type != "UPDATE" {
			t.Fatalf("order event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for order event")
	}
	select {
	case event := <-trades:
		if event.ID != "trade-1" || event.Status != "MATCHED" || event.BucketIndex != 3 || len(event.MakerOrders) != 1 || event.MakerOrders[0].TokenID.String() != testTokenTwo {
			t.Fatalf("trade event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for trade event")
	}

	heartbeat := receiveMap(t, received)
	if heartbeat["heartbeat"] != "PING" {
		t.Fatalf("heartbeat = %#v, want plain text PING", heartbeat)
	}
}

func TestUserClientReconnectRestoresMarkets(t *testing.T) {
	t.Parallel()

	initialMessages := make(chan map[string]any, 2)
	updates := make(chan map[string]any, 1)
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		connectionNumber := connections.Add(1)
		_, message, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var initial map[string]any
		if json.Unmarshal(message, &initial) != nil {
			return
		}
		initialMessages <- initial

		if connectionNumber == 1 {
			_, message, err = conn.ReadMessage()
			if err != nil {
				return
			}
			var update map[string]any
			if json.Unmarshal(message, &update) == nil {
				updates <- update
			}
			return
		}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	conditionOne := mustConditionID(t, testConditionOne)
	conditionTwo := mustConditionID(t, testConditionTwo)
	client := newUserClient(
		strings.Replace(server.URL, "http://", "ws://", 1),
		types.ApiCreds{Key: "key", Secret: "secret", Passphrase: "pass"},
		10*time.Millisecond,
		time.Hour,
	)
	defer client.Stop()

	if err := client.Start([]types.ConditionID{conditionOne}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	first := receiveMap(t, initialMessages)
	assertMarkets(t, first, testConditionOne)

	if err := client.SubscribeMarkets([]types.ConditionID{conditionTwo}); err != nil {
		t.Fatalf("SubscribeMarkets() error = %v", err)
	}
	update := receiveMap(t, updates)
	if update["operation"] != "subscribe" {
		t.Fatalf("update = %#v", update)
	}

	second := receiveMap(t, initialMessages)
	assertMarkets(t, second, testConditionOne, testConditionTwo)
}

func TestUserClientValidationAndAllMarkets(t *testing.T) {
	t.Parallel()

	client := NewUserClient(types.ApiCreds{}, time.Second)
	if err := client.Start(nil); err == nil {
		t.Fatal("Start() with incomplete credentials succeeded")
	}

	received := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, message, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var initial map[string]any
		if json.Unmarshal(message, &initial) == nil {
			received <- initial
		}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	allClient := newUserClient(
		strings.Replace(server.URL, "http://", "ws://", 1),
		types.ApiCreds{Key: "key", Secret: "secret", Passphrase: "pass"},
		time.Second,
		time.Hour,
	)
	defer allClient.Stop()
	if err := allClient.Start(nil); err != nil {
		t.Fatalf("Start(nil) error = %v", err)
	}
	initial := receiveMap(t, received)
	if _, exists := initial["markets"]; exists {
		t.Fatalf("Start(nil) sent markets: %#v", initial)
	}
	if err := allClient.SubscribeMarkets([]types.ConditionID{mustConditionID(t, testConditionOne)}); err == nil {
		t.Fatal("SubscribeMarkets() after Start(nil) succeeded")
	}
}

func TestUserSubscriptionRequestDistinguishesAllFromNoMarkets(t *testing.T) {
	t.Parallel()

	allMarkets, err := json.Marshal(userSubscriptionRequest{
		Auth: types.ApiCreds{Key: "key", Secret: "secret", Passphrase: "pass"},
		Type: "user",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(allMarkets), `"markets"`) {
		t.Fatalf("all-markets request = %s, want markets omitted", allMarkets)
	}

	noMarkets := []string{}
	explicitNone, err := json.Marshal(userSubscriptionRequest{
		Auth:    types.ApiCreds{Key: "key", Secret: "secret", Passphrase: "pass"},
		Type:    "user",
		Markets: &noMarkets,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(explicitNone), `"markets":[]`) {
		t.Fatalf("no-markets request = %s, want explicit empty markets", explicitNone)
	}
}

func mustConditionID(t *testing.T, value string) types.ConditionID {
	t.Helper()
	id, err := types.ParseConditionID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func receiveMap(t *testing.T, messages <-chan map[string]any) map[string]any {
	t.Helper()
	select {
	case message := <-messages:
		return message
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for WebSocket message")
		return nil
	}
}

func assertMarkets(t *testing.T, message map[string]any, want ...string) {
	t.Helper()
	raw, ok := message["markets"].([]any)
	if !ok {
		t.Fatalf("markets = %#v", message["markets"])
	}
	got := make(map[string]bool, len(raw))
	for _, market := range raw {
		got[market.(string)] = true
	}
	if len(got) != len(want) {
		t.Fatalf("markets = %#v, want %#v", got, want)
	}
	for _, market := range want {
		if !got[market] {
			t.Fatalf("markets = %#v, missing %s", got, market)
		}
	}
}
