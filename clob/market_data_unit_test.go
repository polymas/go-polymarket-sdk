package clob

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/polymas/go-polymarket-sdk/types"
)

func TestSanitizeTokenIDs(t *testing.T) {
	// 真实形态的 ERC1155 tokenId（77~78 位十进制 uint256）
	v1 := "78433024518676680431174478322854148606578065650008220678402966840627347604025"
	v2 := "62125911194459356377883400729034971285539783764180402613717505645465050447006"
	v3 := "50346565575310273995396997144874891836871065259829083228393044602519086496922"
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil", nil, []string{}},
		{"empty slice", []string{}, []string{}},
		{"only empty string", []string{""}, []string{}},
		{"only whitespace", []string{"   ", "\t", " \n"}, []string{}},
		{"reject non-numeric", []string{"abc", "0xdead"}, []string{}},
		{"reject all zeros", []string{"0", "00"}, []string{}},
		{"reject too long", []string{strings.Repeat("9", 79)}, []string{}},
		{"trim and dedup", []string{v1, "", v1}, []string{v1}},
		{"trim wraps dedup", []string{"  " + v1 + "  ", v1}, []string{v1}},
		{"mixed bad and good", []string{v1, "", "abc", v2, "0xdead", v1, "0"}, []string{v1, v2}},
		{"preserve order", []string{v2, v1, v2, v3}, []string{v2, v1, v3}},
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
	v1 := "78433024518676680431174478322854148606578065650008220678402966840627347604025"
	v2 := "62125911194459356377883400729034971285539783764180402613717505645465050447006"
	in := []types.BookParams{
		{TokenID: v1, Side: "BUY"},
		{TokenID: "", Side: "BUY"},
		{TokenID: "  ", Side: "SELL"},
		{TokenID: "abc", Side: "BUY"},          // bad format — drop
		{TokenID: "0xdead", Side: "BUY"},       // hex — drop
		{TokenID: "0", Side: "BUY"},            // all-zero — drop
		{TokenID: v1, Side: "BUY"},             // duplicate of first
		{TokenID: v1, Side: "SELL"},            // same token, different side — keep
		{TokenID: " " + v2 + " ", Side: "BUY"}, // trimmed
	}
	got := sanitizeBookParams(in)
	want := []types.BookParams{
		{TokenID: v1, Side: "BUY"},
		{TokenID: v1, Side: "SELL"},
		{TokenID: v2, Side: "BUY"},
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
			baseURL:  url,
			negRisk:  map[string]bool{},
			feeRates: map[string]int{},
		},
	}
}

func TestGetTickSizeDoesNotHideCacheFromBusinessLayer(t *testing.T) {
	var calls atomic.Int32
	fetch := func(tokenID string) (map[string]interface{}, error) {
		if tokenID != "123" {
			t.Fatalf("unexpected token_id: %s", tokenID)
		}
		if calls.Add(1) == 1 {
			return map[string]interface{}{"minimum_tick_size": string(types.TickSize0_01)}, nil
		}
		return map[string]interface{}{"minimum_tick_size": string(types.TickSize0_001)}, nil
	}

	first, err := getTickSizeWithFetcher("123", fetch)
	if err != nil {
		t.Fatalf("first GetTickSize: %v", err)
	}
	second, err := getTickSizeWithFetcher("123", fetch)
	if err != nil {
		t.Fatalf("second GetTickSize: %v", err)
	}
	if first != types.TickSize0_01 || second != types.TickSize0_001 || calls.Load() != 2 {
		t.Fatalf("ticks = %q then %q, calls=%d; want 0.01 then 0.001 with two requests", first, second, calls.Load())
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
		// 每个 id 必须是合法格式且唯一,否则 sanitize 会过滤掉。
		// 用 7843...4025 + 4 位 padding 凑出 501 个唯一的 81 位以下数字串。
		ids[i] = "1" + strings.Repeat("0", 4) + leftPad(i, 4)
	}
	_, err := client.GetMidpoints(ids)
	if err == nil || !strings.Contains(err.Error(), "不能超过500") {
		t.Fatalf("expected length error, got %v", err)
	}
}

// leftPad 把整数 i 左补 0 到 width 位（用于构造唯一 tokenId 测试输入）。
func leftPad(i, width int) string {
	s := strconv.Itoa(i)
	if len(s) >= width {
		return s
	}
	return strings.Repeat("0", width-len(s)) + s
}

// TestGetMidpointsHTTPErrorIncludesPayloadSnippet 验证 SDK 在 HTTP 调用失败时
// 必须把 n= 和 payload 摘要带进 error,便于线上从一行日志定位是哪批入参出问题。
// SDK 的 http 层强制 HTTPS,直接给一个不可达的非 HTTPS baseURL 就能稳定触发
// 失败路径而无需起 TLS 测试服务器。
func TestGetMidpointsHTTPErrorIncludesPayloadSnippet(t *testing.T) {
	client := newReadonlyMarketDataAt("http://invalid.invalid.localhost")
	v1 := "78433024518676680431174478322854148606578065650008220678402966840627347604025"
	v2 := "62125911194459356377883400729034971285539783764180402613717505645465050447006"
	_, err := client.GetMidpoints([]string{v1, v2})
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
	if !strings.Contains(msg, `"token_id":"`+v1+`"`) {
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

// TestGetMultipleOrderBooksAllBadInput 覆盖 /books 的入参清洗。
// 与 GetMidpoints/GetSpreads/GetLastTradesPrices 行为统一：sanitize 后为空
// 直接返回空切片，不发请求也不报错。
func TestGetMultipleOrderBooksAllBadInput(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer server.Close()

	client := newReadonlyMarketDataAt(server.URL)
	for _, tc := range [][]types.BookParams{
		nil,
		{},
		{{TokenID: ""}, {TokenID: "  "}},
		{{TokenID: "abc"}, {TokenID: "0xdead"}, {TokenID: "0"}},
	} {
		got, err := client.GetMultipleOrderBooks(tc)
		if err != nil {
			t.Fatalf("GetMultipleOrderBooks(%v) error: %v", tc, err)
		}
		if len(got) != 0 {
			t.Fatalf("GetMultipleOrderBooks(%v) = %v, want empty slice", tc, got)
		}
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("expected zero HTTP calls, got %d", hits)
	}
}

func TestGetOrderBookDecodesCompleteSnapshotAndNullLastTrade(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/book" || r.URL.Query().Get("token_id") != "42" {
			t.Fatalf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"market":"0x1111111111111111111111111111111111111111111111111111111111111111",
			"asset_id":"42","timestamp":"1234567890","hash":"snapshot-hash",
			"bids":[],"asks":[],"min_order_size":"5.000000",
			"tick_size":"0.001","neg_risk":true,"last_trade_price":""
		}`))
	}))
	defer server.Close()

	book, err := newReadonlyMarketDataAt(server.URL).GetOrderBook("42")
	if err != nil {
		t.Fatalf("GetOrderBook: %v", err)
	}
	if book.TokenID != "42" || book.Market == "" || book.Timestamp != "1234567890" || book.Hash != "snapshot-hash" {
		t.Fatalf("incomplete snapshot: %+v", book)
	}
	if book.MinOrderSize.String() != "5.000000" || book.TickSize != types.TickSize0_001 || !book.NegRisk {
		t.Fatalf("market configuration was not preserved: %+v", book)
	}
	if book.LastTradePrice != nil || book.Bids == nil || book.Asks == nil {
		t.Fatalf("empty/null semantics were not preserved: %+v", book)
	}
}

func TestGetMultipleOrderBooksUsesSamePreciseModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/books" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body []types.BookParams
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if len(body) != 1 || body[0].TokenID != "42" {
			t.Fatalf("body = %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{
			"market":"0x1111111111111111111111111111111111111111111111111111111111111111",
			"asset_id":"42","timestamp":"1234567890","hash":"snapshot-hash",
			"bids":[{"price":"0.123456789012345678","size":"987654321.123456789"}],
			"asks":[{"price":"0.9","size":"5"}],"min_order_size":"5",
			"tick_size":"0.001","neg_risk":false,"last_trade_price":"0.123456789012345678"
		}]`))
	}))
	defer server.Close()

	books, err := newReadonlyMarketDataAt(server.URL).GetMultipleOrderBooks([]types.BookParams{{TokenID: "42"}})
	if err != nil {
		t.Fatalf("GetMultipleOrderBooks: %v", err)
	}
	if len(books) != 1 {
		t.Fatalf("books = %+v", books)
	}
	book := books[0]
	if len(book.Bids) != 1 || book.Bids[0].Price.String() != "0.123456789012345678" ||
		book.Bids[0].Size.String() != "987654321.123456789" {
		t.Fatalf("decimal precision was lost: %+v", book.Bids)
	}
	if book.LastTradePrice == nil || book.LastTradePrice.String() != "0.123456789012345678" {
		t.Fatalf("last trade price = %v", book.LastTradePrice)
	}
	var legacyName types.OrderBookSummaryResponse = book
	if legacyName.TokenID != book.TokenID {
		t.Fatal("OrderBookSummaryResponse compatibility alias changed the value")
	}
}

// TestGetPricesSanitizesInput 验证 GetPrices 也会过滤坏 tokenId。
// 这是历史漏过的端点（修复前 GetPrices 不调用 sanitize），混入 hex/字母/全 0
// 会导致网关的请求体含无意义的 token_id，浪费一次往返。
func TestGetPricesSanitizesInput(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer server.Close()

	// readonly client 没有 GetPrices，直接构造 marketDataClientImpl 测。
	client := &marketDataClientImpl{baseClient: &baseClient{baseURL: server.URL}}
	got, err := client.GetPrices([]types.BookParams{
		{TokenID: "abc", Side: "BUY"},
		{TokenID: "0xdead", Side: "BUY"},
		{TokenID: "0", Side: "BUY"},
		{TokenID: "", Side: "BUY"},
	})
	if err != nil {
		t.Fatalf("GetPrices unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty result, got %v", got)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("expected zero HTTP calls, got %d", hits)
	}
}

// TestIsValidTokenIDFormat 单点验证 ERC1155 tokenId 格式校验。
func TestIsValidTokenIDFormat(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"78433024518676680431174478322854148606578065650008220678402966840627347604025", true},
		{"1", true},
		{"123", true},
		// uint256 max 是 78 位
		{strings.Repeat("9", 78), true},
		{"", false},
		{"0", false},                     // 全 0
		{"00", false},                    // 全 0
		{"0xdead", false},                // hex
		{"abc", false},                   // 字母
		{"-123", false},                  // 负数
		{"1.5", false},                   // 浮点
		{"1 2 3", false},                 // 含空格
		{strings.Repeat("9", 79), false}, // 超过 78 位
	}
	for _, tc := range cases {
		if got := isValidTokenIDFormat(tc.in); got != tc.want {
			t.Errorf("isValidTokenIDFormat(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
