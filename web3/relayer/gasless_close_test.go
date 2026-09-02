package relayer

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/signing"
	"github.com/polymas/go-polymarket-sdk/types"
)

func TestGaslessClientCloseClearsSigningState(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	client, err := NewGaslessClient(
		"1111111111111111111111111111111111111111111111111111111111111111",
		types.SafeSignatureType,
		types.Polygon,
		nil,
		server.URL,
	)
	if err != nil {
		t.Fatal(err)
	}
	client.localSigner.SetV2Key(&internal.V2RelayerKey{
		Key:     "sensitive-relayer-key",
		Address: client.GetBaseAddress().String(),
	})
	signer := client.GetSigner()

	client.Close()
	client.Close()

	if client.localSigner.HasV2Key() {
		t.Fatal("gasless relayer key should be cleared")
	}
	if !signer.Closed() {
		t.Fatal("gasless signer should be closed")
	}
	if _, err := client.localSigner.SignPayload(map[string]interface{}{"method": "GET", "path": "/"}); !errors.Is(err, signing.ErrSignerClosed) {
		t.Fatalf("SignPayload after Close error = %v, want ErrSignerClosed", err)
	}
}
