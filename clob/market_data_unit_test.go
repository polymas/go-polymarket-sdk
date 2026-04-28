package clob

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/polymas/go-polymarket-sdk/types"
)

func TestSanitizeTokenIDs(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil", nil, []string{}},
		{"empty slice", []string{}, []string{}},
		{"only empty string", []string{""}, []string{}},
		{"only whitespace", []string{"   ", "\t", " \n"}, []string{}},
		{"trim and dedup", []string{"abc", "", "abc"}, []string{"abc"}},
		{"trim wraps dedup", []string{"  abc  ", "abc"}, []string{"abc"}},
		{"preserve order", []string{"b", "a", "b", "c"}, []string{"b", "a", "c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeTokenIDs(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("length mismatch: got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("index %d: got %q, want %q (full got=%v want=%v)", i, got[i], tc.want[i], got, tc.want)
				}
			}
		})
	}
}

func TestSanitizeBookParams(t *testing.T) {
	in := []types.BookParams{
		{TokenID: "a", Side: "BUY"},
		{TokenID: "", Side: "BUY"},
		{TokenID: "  ", Side: "SELL"},
		{TokenID: "a", Side: "BUY"},   // duplicate of first
		{TokenID: "a", Side: "SELL"},  // same token, different side — keep
		{TokenID: " b ", Side: "BUY"}, // trimmed to "b"
	}
	got := sanitizeBookParams(in)
	want := []types.BookParams{
		{TokenID: "a", Side: "BUY"},
		{TokenID: "a", Side: "SELL"},
		{TokenID: "b", Side: "BUY"},
	}
	if len(got) != len(want) {
		t.Fatalf("length mismatch: got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("index %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestPayloadSnippet(t *testing.T) {
	short := []byte(`[{"token_id":"abc"}]`)
	if got := payloadSnippet(short); got != string(short) {
		t.Fatalf("short payload should be returned as-is, got %q", got)
	}

	long := make([]byte, 250)
	for i := range long {
		long[i] = 'x'
	}
	got := payloadSnippet(long)
	if !strings.HasSuffix(got, "...(truncated)") {
		t.Fatalf("long payload should be truncated, got suffix %q", got[len(got)-20:])
	}
	if len(got) != 200+len("...(truncated)") {
		t.Fatalf("truncated length unexpected: got %d", len(got))
	}
}

// newReadonlyMarketDataAt 构造一个指向 server.URL 的只读 market data client,
// 用于绕过真实 CLOB API 做端到端的小型测试。
func newReadonlyMarketDataAt(url string) *readonlyMarketDataClientImpl {
	return &readonlyMarketDataClientImpl{
		readonlyBaseClient: &readonlyBaseClient{
			baseURL:   url,
			tickSizes: map[string]types.TickSize{},
			negRisk:   map[string]bool{},
			feeRates:  map[string]int{},
		},
	}
}

// TestGetMidpointsSkipsRequestWhenAllEmpty 验证调用方传入全空 token_id 时,
// SDK 不应该发请求 (这是当前 HTTP 400 报错的根因路径)。
func TestGetMidpointsSkipsRequestWhenAllEmpty(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := newReadonlyMarketDataAt(server.URL)

	for _, tc := range [][]string{
		nil,
		{},
		{""},
		{"", "  ", "\t"},
	} {
		got, err := client.GetMidpoints(tc)
		if err != nil {
			t.Fatalf("GetMidpoints(%v) error: %v", tc, err)
		}
		if len(got) != 0 {
			t.Fatalf("GetMidpoints(%v) = %v, want empty slice", tc, got)
		}
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("expected zero HTTP calls, got %d", hits)
	}
}

// TestGetMidpointsTooMany 保留原有的长度上限保护。
func TestGetMidpointsTooMany(t *testing.T) {
	client := newReadonlyMarketDataAt("http://invalid.invalid")
	ids := make([]string, 501)
	for i := range ids {
		// 每个 id 唯一,避免被去重逻辑压到 500 以下。
		ids[i] = "tok-" + strings.Repeat("a", i+1)
	}
	_, err := client.GetMidpoints(ids)
	if err == nil || !strings.Contains(err.Error(), "不能超过500") {
		t.Fatalf("expected length error, got %v", err)
	}
}

// TestGetMidpointsHTTPErrorIncludesPayloadSnippet 验证 SDK 在 HTTP 调用失败时
// 必须把 n= 和 payload 摘要带进 error,便于线上从一行日志定位是哪批入参出问题。
// SDK 的 http 层强制 HTTPS,直接给一个不可达的非 HTTPS baseURL 就能稳定触发
// 失败路径而无需起 TLS 测试服务器。
func TestGetMidpointsHTTPErrorIncludesPayloadSnippet(t *testing.T) {
	client := newReadonlyMarketDataAt("http://invalid.invalid.localhost")
	_, err := client.GetMidpoints([]string{"abc", "def"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "批量获取中间价失败") {
		t.Errorf("error should keep the operation prefix, got: %s", msg)
	}
	if !strings.Contains(msg, "n=2") {
		t.Errorf("error should include n=2, got: %s", msg)
	}
	if !strings.Contains(msg, `"token_id":"abc"`) {
		t.Errorf("error should include payload snippet with token_id, got: %s", msg)
	}
}

// TestGetSpreadsSkipsRequestWhenAllEmpty 与 GetMidpoints 同样的路径覆盖 /spreads。
func TestGetSpreadsSkipsRequestWhenAllEmpty(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer server.Close()

	client := newReadonlyMarketDataAt(server.URL)
	got, err := client.GetSpreads([]string{"", "   "})
	if err != nil {
		t.Fatalf("GetSpreads error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty result, got %v", got)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("expected zero HTTP calls, got %d", hits)
	}
}

// TestGetLastTradesPricesSkipsRequestWhenAllEmpty 覆盖 /last-trades-prices。
func TestGetLastTradesPricesSkipsRequestWhenAllEmpty(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer server.Close()

	client := newReadonlyMarketDataAt(server.URL)
	got, err := client.GetLastTradesPrices([]string{""})
	if err != nil {
		t.Fatalf("GetLastTradesPrices error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty result, got %v", got)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("expected zero HTTP calls, got %d", hits)
	}
}

// TestGetMultipleOrderBooksRejectsAllEmpty 覆盖 /books 的入参清洗。
// /books 的语义是 "请求数组不能为空", 故全空 token_id 视为空请求,返回明确错误。
func TestGetMultipleOrderBooksRejectsAllEmpty(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer server.Close()

	client := newReadonlyMarketDataAt(server.URL)
	_, err := client.GetMultipleOrderBooks([]types.BookParams{
		{TokenID: ""},
		{TokenID: "  "},
	})
	if err == nil {
		t.Fatal("expected error for all-empty request, got nil")
	}
	if !strings.Contains(err.Error(), "不能为空") {
		t.Fatalf("expected '不能为空' error, got: %v", err)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("expected zero HTTP calls, got %d", hits)
	}
}
