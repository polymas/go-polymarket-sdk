package gamma

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/polymas/go-polymarket-sdk/types"
)

func TestGammaExtendedEndpoints(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("OK")) })
	mux.HandleFunc("/teams", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query()["league"]; len(got) != 2 || got[0] != "nfl" || got[1] != "nba" {
			t.Errorf("league=%v", got)
		}
		_, _ = w.Write([]byte(`[{"id":7,"name":"A","createdAt":"2024-01-01 00:00:00.000+00","updatedAt":"2024-01-02T00:00:00Z"}]`))
	})
	mux.HandleFunc("/tags/3/related-tags", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("omit_empty") != "true" || r.URL.Query().Get("status") != "active" {
			t.Errorf("query=%v", r.URL.Query())
		}
		_, _ = w.Write([]byte(`[{"id":"1","tagID":3,"relatedTagID":4,"rank":2}]`))
	})
	mux.HandleFunc("/events/keyset", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for key, want := range map[string]string{"limit": "500", "locale": "zh", "tag_match": "all", "closed": "false"} {
			if q.Get(key) != want {
				t.Errorf("%s=%q want %q", key, q.Get(key), want)
			}
		}
		if got := q["id"]; len(got) != 2 {
			t.Errorf("id=%v", got)
		}
		_, _ = w.Write([]byte(`{"events":[{"id":"9","slug":"e","title":"E"}],"next_cursor":"next-event"}`))
	})
	mux.HandleFunc("/markets/keyset", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for key, want := range map[string]string{"limit": "100", "decimalized": "true", "rfq_enabled": "false", "tag_match": "any", "locale": "en"} {
			if q.Get(key) != want {
				t.Errorf("%s=%q want %q", key, q.Get(key), want)
			}
		}
		_, _ = w.Write([]byte(`{"markets":[{"id":"11","slug":"m","question":"M","conditionId":"0x0000000000000000000000000000000000000000000000000000000000000000"}],"next_cursor":"next-market"}`))
	})
	mux.HandleFunc("/markets/information", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		var body MarketsInformationRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.ConditionIDs) != 1 || body.ConditionIDs[0] != "0x1" {
			t.Errorf("body=%+v", body)
		}
		_, _ = w.Write([]byte(`[]`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()
	c := &polymarketGammaClient{baseURL: server.URL}

	if status, err := c.GetStatusContext(context.Background()); err != nil || status != "OK" {
		t.Fatalf("status=%q err=%v", status, err)
	}
	teams, err := c.GetTeamsContext(context.Background(), 10, 0, WithTeamLeagues("nfl", "nba"))
	if err != nil || len(teams) != 1 || teams[0].TeamID != 7 || teams[0].CreatedAt.IsZero() {
		t.Fatalf("teams=%+v err=%v", teams, err)
	}
	related, err := c.GetRelatedTagsByIDContext(context.Background(), 3, WithRelatedTagsOmitEmpty(true), WithRelatedTagsStatus("active"))
	if err != nil || len(related) != 1 || related[0].RelatedTagID != 4 {
		t.Fatalf("related=%+v err=%v", related, err)
	}
	closed := false
	events, err := c.GetEventsPageContext(context.Background(), 999, EventsKeysetOptions{IDs: []int{1, 2}, Closed: &closed, TagMatch: "all", Locale: "zh"})
	if err != nil || events.NextCursor != "next-event" || len(events.Events) != 1 {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	markets, err := c.GetMarketsPageContext(context.Background(), 999, WithDecimalized(true), WithRFQEnabled(false), WithTagMatch("any"), WithLocale("en"))
	if err != nil || markets.NextCursor != "next-market" || len(markets.Markets) != 1 {
		t.Fatalf("markets=%+v err=%v", markets, err)
	}
	if _, err := c.GetMarketsInformationContext(context.Background(), MarketsInformationRequest{ConditionIDs: []string{"0x1"}}); err != nil {
		t.Fatal(err)
	}
}

var _ Client = (*polymarketGammaClient)(nil)
var _ = types.EventsPage{}
