package websocket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestSportsClientProtocolAndEvent(t *testing.T) {
	t.Parallel()

	serverErrors := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		if err := conn.WriteMessage(websocket.TextMessage, []byte("ping")); err != nil {
			return
		}
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.TextMessage || string(message) != "pong" {
			serverErrors <- "first client message was not plain text pong: " + string(message)
			return
		}

		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{
			"slug":"sea-sf-2025-02-03","live":true,"ended":false,
			"score":"14-7","period":"Q2","elapsed":"08:45",
			"last_update":"2025-02-03T20:15:30.123Z",
			"finished_timestamp":"","turn":"sea"
		}`))
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	client := newSportsClient(strings.Replace(server.URL, "http://", "ws://", 1), 10*time.Millisecond)
	defer client.Stop()
	results := make(chan *SportResult, 1)
	client.SetOnSportsUpdate(func(result *SportResult) { results <- result })

	if err := client.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	select {
	case message := <-serverErrors:
		t.Fatal(message)
	case result := <-results:
		if result.Slug != "sea-sf-2025-02-03" || !result.Live || result.Ended ||
			result.Score != "14-7" || result.Period != "Q2" || result.Elapsed != "08:45" ||
			result.LastUpdate != "2025-02-03T20:15:30.123Z" || result.Turn != "sea" {
			t.Fatalf("SportResult = %#v", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Sports result")
	}

	if err := client.Start(); err == nil {
		t.Fatal("duplicate Start() succeeded")
	}
}

func TestSportsClientReconnectsWithoutSubscription(t *testing.T) {
	t.Parallel()

	var connections atomic.Int32
	serverErrors := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		connectionNumber := connections.Add(1)

		if err := conn.WriteMessage(websocket.TextMessage, []byte("ping")); err != nil {
			return
		}
		_, message, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if string(message) != "pong" {
			serverErrors <- "unexpected client message: " + string(message)
			return
		}
		if connectionNumber == 1 {
			return
		}
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{
			"slug":"ars-che-2025-02-03","live":false,"ended":true,
			"score":"2-1","period":"FT","elapsed":"",
			"last_update":"2025-02-03T22:00:00.000Z",
			"finished_timestamp":"2025-02-03T21:55:00.000Z"
		}`))
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	client := newSportsClient(strings.Replace(server.URL, "http://", "ws://", 1), 10*time.Millisecond)
	defer client.Stop()
	results := make(chan *SportResult, 1)
	client.SetOnSportsUpdate(func(result *SportResult) { results <- result })
	if err := client.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	select {
	case message := <-serverErrors:
		t.Fatal(message)
	case result := <-results:
		if result.Slug != "ars-che-2025-02-03" || !result.Ended || result.Period != "FT" ||
			result.FinishedTimestamp != "2025-02-03T21:55:00.000Z" {
			t.Fatalf("SportResult after reconnect = %#v", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Sports result after reconnect")
	}
	if connections.Load() < 2 {
		t.Fatalf("connections = %d, want at least 2", connections.Load())
	}
}

func TestSportsClientCanRestart(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	client := newSportsClient(strings.Replace(server.URL, "http://", "ws://", 1), 10*time.Millisecond)
	if err := client.Start(); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	client.Stop()
	if client.IsRunning() {
		t.Fatal("client is running after Stop()")
	}
	if err := client.Start(); err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	client.Stop()
}
