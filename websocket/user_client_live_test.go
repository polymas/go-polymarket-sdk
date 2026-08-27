//go:build live

package websocket

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/polymas/go-polymarket-sdk/clob"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

// TestUserClientLive derives the account's CLOB API credentials and keeps an
// authenticated, read-only User Channel connection alive across a heartbeat.
// It never places, cancels, or otherwise mutates an order.
func TestUserClientLive(t *testing.T) {
	privateKey := os.Getenv("POLY_PRIVATE_KEY")
	if privateKey == "" {
		t.Skip("POLY_PRIVATE_KEY is not set")
	}
	signatureTypeValue, err := strconv.Atoi(envOrDefault("POLY_SIGNATURE_TYPE", "0"))
	if err != nil {
		t.Fatalf("parse POLY_SIGNATURE_TYPE: %v", err)
	}

	web3Client, err := web3.NewClient(
		privateKey,
		types.SignatureType(signatureTypeValue),
		types.Polygon,
	)
	if err != nil {
		t.Fatalf("create Web3 client: %v", err)
	}
	defer web3Client.Close()

	clobClient, err := clob.NewClient(web3Client)
	if err != nil {
		t.Fatalf("create CLOB client: %v", err)
	}
	creds := clobClient.GetAPICreds()
	if creds == nil {
		t.Fatal("CLOB client returned nil API credentials")
	}

	client := NewUserClient(*creds, time.Second).(*userWebSocketClient)
	if err := client.Start(nil); err != nil {
		t.Fatalf("start User Channel: %v", err)
	}
	defer client.Stop()

	initialConn := client.currentConn()
	if initialConn == nil {
		t.Fatal("User Channel has no connection after Start")
	}
	time.Sleep(12 * time.Second)
	if currentConn := client.currentConn(); currentConn == nil || currentConn != initialConn {
		t.Fatal("User Channel did not remain connected across the 10-second heartbeat")
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
