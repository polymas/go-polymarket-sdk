//go:build live

package websocket

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestMarketClientLive performs a public, read-only Market Channel subscription
// and keeps the same connection alive across a client heartbeat interval.
func TestMarketClientLive(t *testing.T) {
	assetID := strings.TrimSpace(os.Getenv("POLY_TEST_TOKEN_ID"))
	if assetID == "" {
		t.Skip("POLY_TEST_TOKEN_ID is required")
	}

	options := DefaultMarketSubscriptionOptions()
	options.CustomFeatureEnabled = true
	marketClient, err := NewClientWithOptions(time.Second, options)
	if err != nil {
		t.Fatalf("create Market Channel client: %v", err)
	}
	client := marketClient.(*webSocketClient)
	events := make(chan MarketEvent, 1)
	client.SetOnMarketEvent(func(event MarketEvent) {
		select {
		case events <- event:
		default:
		}
	})
	if err := client.Start([]string{assetID}); err != nil {
		t.Fatalf("start Market Channel: %v", err)
	}
	defer client.Stop()

	initialConn := client.currentConn()
	if initialConn == nil {
		t.Fatal("Market Channel has no connection after Start")
	}
	select {
	case <-events:
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for a Market Channel event")
	}
	time.Sleep(11 * time.Second)
	if currentConn := client.currentConn(); currentConn == nil || currentConn != initialConn {
		t.Fatal("Market Channel did not remain connected across a heartbeat interval")
	}
}
