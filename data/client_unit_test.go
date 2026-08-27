package data

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/polymas/go-polymarket-sdk/types"
)

func TestGetTradesCurrentQueryContract(t *testing.T) {
	queries := make(chan map[string]string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/trades" {
			t.Errorf("path = %q, want /trades", r.URL.Path)
		}
		query := make(map[string]string)
		for key := range r.URL.Query() {
			query[key] = r.URL.Query().Get(key)
		}
		queries <- query
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()
	client := &polymarketDataClient{baseURL: server.URL}

	if _, err := client.GetTrades(10, 0); err != nil {
		t.Fatal(err)
	}
	defaultQuery := <-queries
	if defaultQuery["takerOnly"] != "true" {
		t.Fatalf("default takerOnly = %q, want true", defaultQuery["takerOnly"])
	}
	if _, ok := defaultQuery["start"]; ok {
		t.Fatalf("default query unexpectedly contains start: %#v", defaultQuery)
	}
	if _, ok := defaultQuery["end"]; ok {
		t.Fatalf("default query unexpectedly contains end: %#v", defaultQuery)
	}

	start := time.Unix(1_700_000_000, 0)
	end := time.Unix(1_700_003_600, 0)
	if _, err := client.GetTrades(20_000, 7,
		WithTradesTakerOnly(false),
		WithTradesDateRange(&start, &end),
	); err != nil {
		t.Fatal(err)
	}
	windowQuery := <-queries
	for key, want := range map[string]string{
		"limit": "10000", "offset": "7", "takerOnly": "false",
		"start": "1700000000", "end": "1700003600",
	} {
		if got := windowQuery[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestSumValueResponsesUsesEveryEntry(t *testing.T) {
	values := []types.ValueResponse{{Value: 1.25}, {Value: 2.75}, {Value: -0.5}}
	if got := sumValueResponses(values); got != 3.5 {
		t.Fatalf("sumValueResponses() = %v, want 3.5", got)
	}
	if got := sumValueResponses(nil); got != 0 {
		t.Fatalf("sumValueResponses(nil) = %v, want 0", got)
	}
}

func TestActivityTypedFilters(t *testing.T) {
	opts := &GetActivityOptions{}
	WithActivityType([]types.ActivityType{
		types.ActivityTypeDeposit,
		types.ActivityTypeWithdrawal,
	})(opts)
	WithActivityExcludeDepositsWithdrawals(false)(opts)

	wantTypes := []types.ActivityType{types.ActivityTypeDeposit, types.ActivityTypeWithdrawal}
	if !reflect.DeepEqual(opts.ActivityType, wantTypes) {
		t.Fatalf("ActivityType = %#v, want %#v", opts.ActivityType, wantTypes)
	}
	if opts.ExcludeDepositsWithdrawals == nil || *opts.ExcludeDepositsWithdrawals {
		t.Fatalf("ExcludeDepositsWithdrawals = %v, want false", opts.ExcludeDepositsWithdrawals)
	}
}
