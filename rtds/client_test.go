package rtds

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/polymas/go-polymarket-sdk/types"
)

func TestProtocolHeartbeatAndExactTWAP(t *testing.T) {
	upgrader := websocket.Upgrader{}
	messages := make(chan []byte, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for i := 0; i < 2; i++ {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}
			messages <- message
			if i == 0 {
				_ = conn.WriteJSON(map[string]any{"topic": "crypto_prices_twap_thirty", "type": "update", "timestamp": 2000, "payload": map[string]any{"symbol": "btc/usd", "value": 65000.5, "full_accuracy_value": "65000500000000000000000", "timestamp": 1000, "window_s": 30}})
			}
		}
	}))
	defer server.Close()

	client := NewClient(10*time.Millisecond, WithURL("ws"+strings.TrimPrefix(server.URL, "http")), WithHeartbeatInterval(10*time.Millisecond))
	updates := make(chan *types.ChainlinkTWAPEvent, 1)
	client.SetOnTWAPUpdate(func(update *types.ChainlinkTWAPEvent) { updates <- update })
	if err := client.Subscribe([]Subscription{{Window: TWAP30Seconds, Symbol: "btc/usd"}, {Window: TWAP60Seconds}}); err != nil {
		t.Fatal(err)
	}
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	defer client.Stop()

	select {
	case message := <-messages:
		want := `"filters":"{\"symbol\":\"btc/usd\"}"`
		if !strings.Contains(string(message), `"action":"subscribe"`) || !strings.Contains(string(message), want) || strings.Contains(string(message), `"window_s"`) {
			t.Fatalf("subscription = %s", message)
		}
	case <-time.After(time.Second):
		t.Fatal("subscription timeout")
	}
	select {
	case update := <-updates:
		if update.Timestamp != 2000 || update.Payload.Timestamp != 1000 || update.Payload.ExactValue != "65000.5" {
			t.Fatalf("update = %+v", update)
		}
	case <-time.After(time.Second):
		t.Fatal("update timeout")
	}
	select {
	case message := <-messages:
		if string(message) != "PING" {
			t.Fatalf("heartbeat = %q", message)
		}
	case <-time.After(time.Second):
		t.Fatal("heartbeat timeout")
	}
}

func TestFormatSignedE18(t *testing.T) {
	tests := map[string]string{"0": "0", "1": "0.000000000000000001", "-1000000000000000001": "-1.000000000000000001"}
	for input, want := range tests {
		got, err := types.FormatSignedE18(input)
		if err != nil || got != want {
			t.Fatalf("FormatSignedE18(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
}

func TestReconnectRestoresSubscriptions(t *testing.T) {
	upgrader := websocket.Upgrader{}
	restored := make(chan struct{}, 1)
	connection := 0
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		mu.Lock()
		connection++
		current := connection
		mu.Unlock()
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var frame struct {
			Action        string           `json:"action"`
			Subscriptions []map[string]any `json:"subscriptions"`
		}
		if json.Unmarshal(payload, &frame) != nil || frame.Action != "subscribe" || len(frame.Subscriptions) != 1 {
			return
		}
		if current == 1 {
			return
		}
		restored <- struct{}{}
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()

	client := NewClient(5*time.Millisecond, WithURL("ws"+strings.TrimPrefix(server.URL, "http")))
	if err := client.Subscribe([]Subscription{{Window: TWAP60Seconds, Symbol: "eth/usd"}}); err != nil {
		t.Fatal(err)
	}
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	defer client.Stop()
	select {
	case <-restored:
	case <-time.After(time.Second):
		t.Fatal("subscription was not restored after reconnect")
	}
}
