package clob

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/polymas/go-polymarket-sdk/types"
)

func TestNegRiskCacheMetadataAndInvalidate(t *testing.T) {
	cache := newNegRiskCache()
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	cache.now = func() time.Time { return now }

	first := cache.set("1", true, negRiskSourceManual)
	second := cache.set("1", false, negRiskSourceOrderArg)
	if first.Version != 1 || second.Version != 2 {
		t.Fatalf("versions = %d,%d", first.Version, second.Version)
	}
	if second.FetchedAt != now || second.Source != negRiskSourceOrderArg || second.Value {
		t.Fatalf("entry = %+v", second)
	}
	cache.invalidate("1")
	if _, ok := cache.get("1"); ok {
		t.Fatal("entry remains after invalidate")
	}
}

func TestNegRiskCacheSingleflight(t *testing.T) {
	cache := newNegRiskCache()
	var fetches atomic.Int32
	fetchStarted := make(chan struct{})
	releaseFetch := make(chan struct{})
	fetch := func() (bool, error) {
		if fetches.Add(1) == 1 {
			close(fetchStarted)
		}
		<-releaseFetch
		return true, nil
	}

	const callers = 32
	results := make(chan bool, callers)
	errors := make(chan error, callers)
	go func() {
		value, err := cache.getOrFetch("1", fetch)
		results <- value
		errors <- err
	}()
	<-fetchStarted

	var ready sync.WaitGroup
	ready.Add(callers - 1)
	start := make(chan struct{})
	for i := 1; i < callers; i++ {
		go func() {
			ready.Done()
			<-start
			value, err := cache.getOrFetch("1", fetch)
			results <- value
			errors <- err
		}()
	}
	ready.Wait()
	close(start)
	time.Sleep(10 * time.Millisecond)
	close(releaseFetch)

	for i := 0; i < callers; i++ {
		if err := <-errors; err != nil {
			t.Fatalf("caller %d: %v", i, err)
		}
		if value := <-results; !value {
			t.Fatalf("caller %d got false", i)
		}
	}
	if got := fetches.Load(); got != 1 {
		t.Fatalf("fetches = %d, want 1", got)
	}
}

func TestNegRiskPrimeAndInvalidate(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Query().Get("token_id") != "1" {
			t.Fatalf("token_id = %q", r.URL.Query().Get("token_id"))
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"neg_risk": false})
	}))
	defer server.Close()

	client := newReadonlyMarketDataAt(server.URL)
	if err := client.PrimeNegRisk("0001", true); err != nil {
		t.Fatalf("PrimeNegRisk: %v", err)
	}
	if value, err := client.GetNegRisk("1"); err != nil || !value {
		t.Fatalf("primed GetNegRisk = %v, %v", value, err)
	}
	if hits.Load() != 0 {
		t.Fatalf("primed lookup made %d requests", hits.Load())
	}

	if err := client.InvalidateNegRisk("1"); err != nil {
		t.Fatalf("InvalidateNegRisk: %v", err)
	}
	if value, err := client.GetNegRisk("1"); err != nil || value {
		t.Fatalf("refetched GetNegRisk = %v, %v", value, err)
	}
	if hits.Load() != 1 {
		t.Fatalf("requests after invalidate = %d, want 1", hits.Load())
	}
}

func TestConcurrentNegRiskBatchPrefetchAndUpdates(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]bool{"neg_risk": r.URL.Query().Get("token_id") == "2"})
	}))
	defer server.Close()

	base := &baseClient{baseURL: server.URL, negRisk: newNegRiskCache()}
	args := []types.OrderArgs{{TokenID: "1"}, {TokenID: "2"}}
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			base.prefetchNegRisk(args)
			base.negRisk.set("1", i%2 == 0, negRiskSourceOrderArg)
			_, _ = base.negRisk.get("2")
		}(i)
	}
	wg.Wait()
	if hits.Load() > 2 {
		t.Fatalf("HTTP requests = %d, want at most one per token", hits.Load())
	}
}
