package rfq

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/polymas/go-polymarket-sdk/signing"
	"github.com/polymas/go-polymarket-sdk/types"
)

func TestGetComboMarkets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/rfq/combo-markets" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("limit"); got != "25" {
			t.Errorf("limit = %s", got)
		}
		if got := r.URL.Query().Get("cursor"); got != "next" {
			t.Errorf("cursor = %s", got)
		}
		if got := r.URL.Query().Get("exclude"); got != "a,b" {
			t.Errorf("exclude = %s", got)
		}
		_, _ = w.Write([]byte(`{"markets":[{"id":"1","condition_id":"c","position_ids":["y","n"],"slug":"s","title":"t","outcomes":["Yes","No"],"outcome_prices":[".5",".5"],"image":"","volume":1,"tags":[]}],"next_cursor":null}`))
	}))
	defer server.Close()

	result, err := newClient(server.URL, Config{}).GetComboMarkets(25, "next", []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Markets) != 1 || result.NextCursor != nil {
		t.Fatalf("unexpected response: %+v", result)
	}
}

func TestMakerRESTRequiresCredentials(t *testing.T) {
	_, err := NewClient(Config{}).SubmitQuote(Quote{})
	if err == nil || !strings.Contains(err.Error(), "requires signer") {
		t.Fatalf("error = %v", err)
	}
}

func TestMakerRESTUsesL2HeadersAndConfiguredIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/maker/quotes" {
			t.Errorf("path = %s", r.URL.Path)
		}
		for _, header := range []string{"POLY_API_KEY", "POLY_ADDRESS", "POLY_SIGNATURE", "POLY_PASSPHRASE", "POLY_TIMESTAMP"} {
			if r.Header.Get(header) == "" {
				t.Errorf("missing %s", header)
			}
		}
		body, _ := io.ReadAll(r.Body)
		var quote Quote
		if err := json.Unmarshal(body, &quote); err != nil {
			t.Fatal(err)
		}
		if quote.SignerAddress != "configured-signer" || quote.MakerAddress != "configured-maker" || quote.PriceE6 != "500000" {
			t.Errorf("quote = %+v", quote)
		}
		_, _ = w.Write([]byte(`{"request":{"rfq_id":"r1","leg_position_ids":[],"direction":"BUY","side":"YES"},"status":"COLLECTING_QUOTES"}`))
	}))
	defer server.Close()
	signer, err := signing.NewSigner(strings.Repeat("1", 64), types.Polygon)
	if err != nil {
		t.Fatal(err)
	}
	client := newClient(server.URL, Config{
		Signer:      signer,
		Credentials: &types.ApiCreds{Key: "key", Secret: "c2VjcmV0", Passphrase: "pass"},
		Identity:    Identity{SignerAddress: "configured-signer", MakerAddress: "configured-maker", SignatureType: types.SafeSignatureType},
	})
	result, err := client.SubmitQuote(Quote{RFQID: "r1", QuoteID: "q1", PriceE6: "500000", SizeE6: "1000000"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "COLLECTING_QUOTES" {
		t.Fatalf("result = %+v", result)
	}
}

func TestRFQWebSocketAuthEventAndQuote(t *testing.T) {
	upgrader := websocket.Upgrader{}
	received := make(chan map[string]any, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for i := 0; i < 2; i++ {
			var message map[string]any
			if err := conn.ReadJSON(&message); err != nil {
				return
			}
			received <- message
			if i == 0 {
				_ = conn.WriteJSON(map[string]any{"type": "RFQ_REQUEST", "rfq_id": "r1"})
			}
		}
	}))
	defer server.Close()

	config := Config{
		Credentials: &types.ApiCreds{Key: "key", Secret: "secret", Passphrase: "pass"},
		Identity:    Identity{SignerAddress: "signer", MakerAddress: "maker", SignatureType: types.EOASignatureType},
	}
	client := NewWebSocketClient(config, 10*time.Millisecond, WithWebSocketURL("ws"+strings.TrimPrefix(server.URL, "http")))
	events := make(chan Event, 1)
	client.SetOnEvent(func(event Event) { events <- event })
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	defer client.Stop()

	select {
	case auth := <-received:
		if auth["type"] != "auth" {
			t.Fatalf("auth = %#v", auth)
		}
		identity := auth["identity"].(map[string]any)
		if identity["maker_address"] != "maker" {
			t.Fatalf("identity = %#v", identity)
		}
	case <-time.After(time.Second):
		t.Fatal("auth timeout")
	}
	select {
	case event := <-events:
		if event.Type != "RFQ_REQUEST" || !json.Valid(event.Payload) {
			t.Fatalf("event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("event timeout")
	}
	if err := client.SendQuote(WSQuote{RFQID: "r1", PriceE6: "500000", SizeE6: "1000000"}); err != nil {
		t.Fatal(err)
	}
	select {
	case quote := <-received:
		if quote["type"] != "RFQ_QUOTE" || quote["rfq_id"] != "r1" {
			t.Fatalf("quote = %#v", quote)
		}
	case <-time.After(time.Second):
		t.Fatal("quote timeout")
	}
}

func TestRFQWebSocketReconnectsWithoutResendingQuote(t *testing.T) {
	upgrader := websocket.Upgrader{}
	var mu sync.Mutex
	connections := 0
	firstQuote := make(chan struct{}, 1)
	verified := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		mu.Lock()
		connections++
		current := connections
		mu.Unlock()
		var auth map[string]any
		if conn.ReadJSON(&auth) != nil || auth["type"] != "auth" {
			return
		}
		if current == 1 {
			_ = conn.WriteJSON(map[string]any{"type": "RFQ_REQUEST", "rfq_id": "r1"})
			var quote map[string]any
			if conn.ReadJSON(&quote) == nil && quote["type"] == "RFQ_QUOTE" {
				firstQuote <- struct{}{}
			}
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		if _, _, err := conn.ReadMessage(); err != nil {
			verified <- struct{}{}
		}
	}))
	defer server.Close()
	client := NewWebSocketClient(Config{
		Credentials: &types.ApiCreds{Key: "key", Secret: "secret", Passphrase: "pass"},
		Identity:    Identity{SignerAddress: "signer", MakerAddress: "maker"},
	}, 5*time.Millisecond, WithWebSocketURL("ws"+strings.TrimPrefix(server.URL, "http")))
	request := make(chan struct{}, 1)
	client.SetOnEvent(func(event Event) {
		if event.Type == "RFQ_REQUEST" {
			request <- struct{}{}
		}
	})
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	defer client.Stop()
	select {
	case <-request:
	case <-time.After(time.Second):
		t.Fatal("request timeout")
	}
	if err := client.SendQuote(WSQuote{RFQID: "r1", PriceE6: "1", SizeE6: "1"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstQuote:
	case <-time.After(time.Second):
		t.Fatal("first quote timeout")
	}
	select {
	case <-verified:
	case <-time.After(time.Second):
		t.Fatal("reconnect/no-resend verification timeout")
	}
}
