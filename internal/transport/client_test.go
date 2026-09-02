package transport

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdkerrors "github.com/polymas/go-polymarket-sdk/errors"
)

func TestAPIErrorFieldsAndRedaction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-Request-ID", "request-123")
		writer.Header().Set("Retry-After", "3")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = writer.Write([]byte(`{"error":{"code":"RATE_LIMIT","message":"slow down"},"api_secret":"secret-value","signature":"0xdead"}`))
	}))
	defer server.Close()

	_, err := Get[map[string]any](server.URL, "/orders", nil, WithoutRetry(), WithService("clob"))
	var apiErr *sdkerrors.APIError
	if !stderrors.As(err, &apiErr) {
		t.Fatalf("got %T %v, want APIError", err, err)
	}
	if apiErr.Service != "clob" || apiErr.Method != "GET" || apiErr.Path != "/orders" || apiErr.Status != 429 || apiErr.RequestID != "request-123" || apiErr.ErrorCode != "RATE_LIMIT" || apiErr.Message != "slow down" || apiErr.RetryAfter != 3*time.Second || !apiErr.Retryable {
		t.Fatalf("unexpected APIError: %+v", apiErr)
	}
	if strings.Contains(apiErr.RawResponse, "secret-value") || strings.Contains(apiErr.RawResponse, "0xdead") {
		t.Fatalf("raw response leaked credentials: %s", apiErr.RawResponse)
	}
}

func TestGETRetries425429503AndPreservesLargeSuccess(t *testing.T) {
	var calls atomic.Int32
	large := strings.Repeat("x", 8<<10)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		switch calls.Add(1) {
		case 1:
			writer.WriteHeader(http.StatusTooEarly)
		case 2:
			writer.WriteHeader(http.StatusServiceUnavailable)
		default:
			_, _ = fmt.Fprintf(writer, `{"value":%q}`, large)
		}
	}))
	defer server.Close()

	result, err := Get[struct {
		Value string `json:"value"`
	}](server.URL, "/retry", nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 || result.Value != large {
		t.Fatalf("calls=%d len(value)=%d", calls.Load(), len(result.Value))
	}
}

func TestPOSTIsNotRetriedUnlessExplicitlyIdempotent(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	_, err := Post[map[string]any](server.URL, "/mutate", map[string]any{"x": 1})
	if err == nil || calls.Load() != 1 {
		t.Fatalf("unsafe POST err=%v calls=%d, want one attempt", err, calls.Load())
	}
	calls.Store(0)
	_, err = Post[map[string]any](server.URL, "/read-batch", []string{"1"}, WithIdempotent(), WithMaxRetries(2))
	if err == nil || calls.Load() != 3 {
		t.Fatalf("idempotent POST err=%v calls=%d, want three attempts", err, calls.Load())
	}
}

func TestContextCancellationAndAmbiguousMutation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = writer.Write([]byte(`{}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := GetContext[map[string]any](ctx, server.URL, "/read", nil)
	if !stderrors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("GET cancellation = %v", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = PostRawContext(ctx, server.URL, "/orders", []byte(`{}`), WithAmbiguousOnTimeout("place orders"))
	var ambiguous *sdkerrors.AmbiguousOutcomeError
	if !stderrors.As(err, &ambiguous) || !sdkerrors.IsAmbiguousOutcome(err) {
		t.Fatalf("POST timeout = %T %v, want AmbiguousOutcomeError", err, err)
	}
	if ambiguous.Operation != "place orders" || !stderrors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unexpected ambiguous error: %+v", ambiguous)
	}
}

func TestCanceledBeforeMutationIsNotAmbiguous(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := PostRawContext(ctx, "https://clob.polymarket.com", "/orders", []byte(`{}`), WithAmbiguousOnTimeout("place orders"))
	if !stderrors.Is(err, context.Canceled) || sdkerrors.IsAmbiguousOutcome(err) {
		t.Fatalf("error=%T %v, want known pre-submit cancellation", err, err)
	}
}

func TestMultiParamsAndContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := request.URL.Query()["token_id"]; len(got) != 2 || got[0] != "1" || got[1] != "2" {
			t.Errorf("token_id params = %v", got)
		}
		_, _ = writer.Write([]byte(`[]`))
	}))
	defer server.Close()
	if _, err := GetSliceContext[string](context.Background(), server.URL, "/books", nil, WithMultiParams(map[string][]string{"token_id": {"1", "2"}})); err != nil {
		t.Fatal(err)
	}
}
