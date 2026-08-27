//go:build live

package websocket

import (
	"testing"
	"time"
)

// TestSportsClientLive keeps the public Sports Channel connected across two
// server ping intervals. It performs no authenticated or state-changing work.
func TestSportsClientLive(t *testing.T) {
	client := NewSportsClient(time.Second).(*sportsWebSocketClient)
	if err := client.Start(); err != nil {
		t.Fatalf("start Sports Channel: %v", err)
	}
	defer client.Stop()

	initialConn := client.currentConn()
	if initialConn == nil {
		t.Fatal("Sports Channel has no connection after Start")
	}
	time.Sleep(12 * time.Second)
	if currentConn := client.currentConn(); currentConn == nil || currentConn != initialConn {
		t.Fatal("Sports Channel did not remain connected after responding to server pings")
	}
}
