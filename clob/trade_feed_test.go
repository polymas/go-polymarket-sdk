package clob

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

func tradeFeedTestServer(t *testing.T, calls *atomic.Int32, txHash string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != internal.Trades {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"limit": 1, "next_cursor": internal.EndCursor, "count": 1,
			"data": []map[string]interface{}{{
				"id": r.URL.Query().Get("id"), "status": "CONFIRMED", "transaction_hash": txHash,
			}},
		})
	}))
}

func TestWaitForTradeIDsPrefersFeedOverPolling(t *testing.T) {
	var calls atomic.Int32
	server := tradeFeedTestServer(t, &calls, testTxHash)
	defer server.Close()
	client := newAuthenticatedOrderClientForTest(t, server.URL)

	// 先喂一个未结算事件激活 feed（模拟 WS 已连上并收到 MATCHED）。
	client.RecordTradeUpdate(types.ClobTrade{ID: "trade-1", Status: "MATCHED"})
	go func() {
		time.Sleep(30 * time.Millisecond)
		client.RecordTradeUpdate(types.ClobTrade{ID: "trade-1", Status: "CONFIRMED", TransactionHash: types.Keccak256(testTxHash)})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	trades, timing, err := client.waitForTradeIDs(ctx, []string{"trade-1"}, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("waitForTradeIDs: %v", err)
	}
	if got := trades["trade-1"].TransactionHash; got != types.Keccak256(testTxHash) {
		t.Fatalf("transaction hash = %q, want %q", got, testTxHash)
	}
	if calls.Load() != 0 || timing.PollCount != 0 {
		t.Fatalf("HTTP polls = %d (timing %d), want 0 when the feed resolves the trade", calls.Load(), timing.PollCount)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("woke up after %v, want prompt wake-up on feed event", elapsed)
	}
}

func TestWaitForTradeIDsFallsBackToHTTPWhenFeedSilent(t *testing.T) {
	prev := tradeFeedFallbackPollInterval
	tradeFeedFallbackPollInterval = 40 * time.Millisecond
	t.Cleanup(func() { tradeFeedFallbackPollInterval = prev })

	var calls atomic.Int32
	server := tradeFeedTestServer(t, &calls, testTxHash)
	defer server.Close()
	client := newAuthenticatedOrderClientForTest(t, server.URL)
	client.RecordTradeUpdate(types.ClobTrade{ID: "unrelated", Status: "MATCHED"}) // 激活 feed 但不覆盖目标 trade

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	trades, timing, err := client.waitForTradeIDs(ctx, []string{"trade-2"}, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("waitForTradeIDs: %v", err)
	}
	if trades["trade-2"].TransactionHash != types.Keccak256(testTxHash) {
		t.Fatalf("trade not resolved via HTTP fallback: %+v", trades["trade-2"])
	}
	if calls.Load() != 1 || timing.PollCount != 1 {
		t.Fatalf("HTTP polls = %d (timing %d), want exactly one fallback poll", calls.Load(), timing.PollCount)
	}
	if elapsed := time.Since(start); elapsed < 30*time.Millisecond {
		t.Fatalf("polled after %v, want the first HTTP poll deferred to the fallback interval", elapsed)
	}
}

func TestWaitForTradeIDsWithoutFeedPollsImmediately(t *testing.T) {
	var calls atomic.Int32
	server := tradeFeedTestServer(t, &calls, testTxHash)
	defer server.Close()
	client := newAuthenticatedOrderClientForTest(t, server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	if _, timing, err := client.waitForTradeIDs(ctx, []string{"trade-3"}, 250*time.Millisecond); err != nil || timing.PollCount != 1 {
		t.Fatalf("err=%v polls=%d, want one immediate poll", err, timing.PollCount)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("took %v, want immediate poll when no feed is active", elapsed)
	}
}

func TestTradeFeedDoesNotRegressResolvedTrade(t *testing.T) {
	feed := newTradeFeed()
	feed.record(types.ClobTrade{ID: "t", Status: "CONFIRMED", TransactionHash: types.Keccak256(testTxHash)})
	feed.record(types.ClobTrade{ID: "t", Status: "MATCHED"}) // 乱序晚到的早期事件
	got, ok := feed.lookup("t")
	if !ok || got.TransactionHash != types.Keccak256(testTxHash) {
		t.Fatalf("resolved trade was overwritten by a stale event: %+v", got)
	}
	feed.record(types.ClobTrade{ID: ""}) // 空 ID 忽略
	if _, ok := feed.lookup(""); ok {
		t.Fatal("empty trade ID must not be stored")
	}
}
