package middleware

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryMiddlewareDoesNotRetryPOST(t *testing.T) {
	var calls atomic.Int32
	next := func(context.Context, *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("retry"))}, nil
	}
	req, _ := http.NewRequest(http.MethodPost, "https://example.com/orders", strings.NewReader("{}"))
	resp, err := NewRetryMiddleware(2, time.Millisecond, time.Millisecond)(next)(context.Background(), req)
	if err != nil || resp.StatusCode != http.StatusServiceUnavailable || calls.Load() != 1 {
		t.Fatalf("resp=%v err=%v calls=%d", resp, err, calls.Load())
	}
}

func TestRetryMiddlewareRetriesGETAndKeepsFinalBodyOpen(t *testing.T) {
	var calls atomic.Int32
	next := func(context.Context, *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: http.StatusTooEarly, Body: io.NopCloser(strings.NewReader("final"))}, nil
	}
	req, _ := http.NewRequest(http.MethodGet, "https://example.com/book", nil)
	resp, err := NewRetryMiddleware(2, time.Millisecond, time.Millisecond)(next)(context.Background(), req)
	if err != nil || calls.Load() != 3 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil || string(body) != "final" {
		t.Fatalf("body=%q err=%v", body, readErr)
	}
}

func TestRedactedURL(t *testing.T) {
	u, _ := url.Parse("https://example.com/path?api_key=key&signature=0xsig&token_id=123")
	got := redactedURL(u)
	parsed, _ := url.Parse(got)
	if parsed.Query().Get("api_key") != "***" || parsed.Query().Get("signature") != "***" || parsed.Query().Get("token_id") != "123" {
		t.Fatalf("redacted URL=%s", got)
	}
}
