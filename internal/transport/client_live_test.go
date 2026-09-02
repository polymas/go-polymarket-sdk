package transport

import (
	"context"
	stderrors "errors"
	"os"
	"testing"
	"time"

	sdkerrors "github.com/polymas/go-polymarket-sdk/errors"
)

// Read-only production contract test. It never authenticates or mutates state.
func TestPublicCLOBContextAndAPIErrorLive(t *testing.T) {
	if os.Getenv("POLYMARKET_LIVE_TEST") == "" {
		t.Skip("set POLYMARKET_LIVE_TEST=1 for read-only production verification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if body, err := GetRawContext(ctx, "https://clob.polymarket.com", "GET", "/time", nil, WithService("clob")); err != nil || len(body) == 0 {
		t.Fatalf("GET /time body=%q err=%v", body, err)
	}
	_, err := GetRawContext(ctx, "https://clob.polymarket.com", "GET", "/book", map[string]string{"token_id": "not-a-token"}, WithoutRetry(), WithService("clob"))
	var apiErr *sdkerrors.APIError
	if !stderrors.As(err, &apiErr) || apiErr.Service != "clob" || apiErr.Method != "GET" || apiErr.Path != "/book" || apiErr.Status < 400 {
		t.Fatalf("invalid token error=%T %v", err, err)
	}
}
