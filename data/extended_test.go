package data

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/polymas/go-polymarket-sdk/types"
)

func TestDataExtendedEndpoints(t *testing.T) {
	user := types.EthAddress("0x1111111111111111111111111111111111111111")
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"data":"OK"}`)) })
	mux.HandleFunc("/v1/accounting/snapshot", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("user") != string(user) {
			t.Errorf("user=%s", r.URL.Query().Get("user"))
		}
		_, _ = w.Write([]byte("PK\x03\x04zip"))
	})
	mux.HandleFunc("/v1/approvals", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"address":"0x1111111111111111111111111111111111111111","chainId":137,"checkedAt":"now","contracts":[{"id":"x","approved":true}]}`))
	})
	mux.HandleFunc("/v1/activity/combos", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("limit") != "500" || q.Get("offset") != "10000" || len(q["market_id"]) != 2 {
			t.Errorf("combo activity query=%v", q)
		}
		_, _ = w.Write([]byte(`{"activity":[],"pagination":{"limit":500,"offset":10000,"has_more":false,"next_cursor":""}}`))
	})
	mux.HandleFunc("/v1/positions/combos", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("sort") != "current_value_desc" || len(q["status"]) != 2 {
			t.Errorf("combo positions query=%v", q)
		}
		_, _ = w.Write([]byte(`{"combos":[],"pagination":{"limit":20,"offset":0,"has_more":false,"next_cursor":""}}`))
	})
	mux.HandleFunc("/holders", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query()["market"]; len(got) != 2 || got[1] != "c2" {
			t.Errorf("market=%v", got)
		}
		_, _ = w.Write([]byte(`[{"token":"1","holders":[{"proxyWallet":"0x1111111111111111111111111111111111111111","asset":"1","amount":2.5}]}]`))
	})
	mux.HandleFunc("/v1/builders/leaderboard", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("timePeriod") != "DAY" || q.Get("limit") != "50" || q.Get("offset") != "1000" {
			t.Errorf("builder query=%v", q)
		}
		_, _ = w.Write([]byte(`[{"rank":"1","builder":"b","volume":3}]`))
	})
	mux.HandleFunc("/positions", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("includeArchived") != "true" {
			t.Errorf("positions query=%v", r.URL.Query())
		}
		_, _ = w.Write([]byte(`[{"proxyWallet":"0x1111111111111111111111111111111111111111","asset":"1","conditionId":"0x0000000000000000000000000000000000000000000000000000000000000001","grossInitialValue":12.5,"entryFeesUsdc":0.2,"endDate":""}]`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()
	c := &polymarketDataClient{baseURL: server.URL}
	if health, err := c.GetHealthContext(context.Background()); err != nil || health.Data != "OK" {
		t.Fatalf("health=%+v err=%v", health, err)
	}
	if snapshot, err := c.GetAccountingSnapshotContext(context.Background(), user); err != nil || string(snapshot) != "PK\x03\x04zip" {
		t.Fatalf("snapshot=%q err=%v", snapshot, err)
	}
	if approvals, err := c.GetApprovalsContext(context.Background(), user); err != nil || len(approvals.Contracts) != 1 {
		t.Fatalf("approvals=%+v err=%v", approvals, err)
	}
	if _, err := c.GetComboActivityContext(context.Background(), user, ComboActivityOptions{MarketIDs: []string{"m1", "m2"}, Limit: 999, Offset: 99999}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetComboPositionsContext(context.Background(), user, ComboPositionsOptions{Statuses: []string{"OPEN", "PARTIAL"}}); err != nil {
		t.Fatal(err)
	}
	holders, err := c.GetHoldersContext(context.Background(), []string{"c1", "c2"}, 20, 1)
	if err != nil || len(holders) != 1 || len(holders[0].Holders) != 1 {
		t.Fatalf("holders=%+v err=%v", holders, err)
	}
	builders, err := c.GetBuilderLeaderboardContext(context.Background(), BuilderLeaderboardOptions{Limit: 100, Offset: 2000})
	if err != nil || len(builders) != 1 {
		t.Fatalf("builders=%+v err=%v", builders, err)
	}
	positions, err := c.GetPositionsContext(context.Background(), user, WithPositionsIncludeArchived(true))
	if err != nil || len(positions) != 1 || positions[0].GrossInitialValue != 12.5 || positions[0].EntryFeesUSDC != 0.2 {
		t.Fatalf("positions=%+v err=%v", positions, err)
	}
}

var _ Client = (*polymarketDataClient)(nil)
