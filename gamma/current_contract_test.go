package gamma

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/polymas/go-polymarket-sdk/types"
)

func TestCurrentGammaPathsAndShapes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/public-profile":
			if r.URL.Query().Get("address") != "0x1111111111111111111111111111111111111111" {
				t.Errorf("profile query = %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"proxyWallet":"0xproxy","name":"maker"}`))
		case "/series-summary/slug/weekly":
			_, _ = w.Write([]byte(`{"id":"7","slug":"weekly","eventDates":[],"eventWeeks":[]}`))
		case "/series-summary/7":
			_, _ = w.Write([]byte(`{"id":"7","eventDates":[],"eventWeeks":[]}`))
		case "/comments":
			if r.URL.Query().Get("parent_entity_type") != "market" || r.URL.Query().Get("parent_entity_id") != "42" {
				t.Errorf("comments query = %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`[{"id":"9","body":"ok","parentEntityID":42}]`))
		case "/comments/9":
			_, _ = w.Write([]byte(`[{"id":"9"}]`))
		case "/markets/keyset":
			if r.URL.Query().Get("limit") != "100" {
				t.Errorf("keyset limit = %s", r.URL.Query().Get("limit"))
			}
			if r.URL.Query().Get("after_cursor") == "next" {
				_, _ = w.Write([]byte(`{"markets":[],"next_cursor":""}`))
			} else {
				_, _ = w.Write([]byte(`{"markets":[{"id":"1"}],"next_cursor":"next"}`))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := &polymarketGammaClient{baseURL: server.URL}

	profile, err := c.GetProfile(types.EthAddress("0x1111111111111111111111111111111111111111"))
	if err != nil || profile.Name == nil || *profile.Name != "maker" {
		t.Fatalf("profile = %+v, %v", profile, err)
	}
	if summary, err := c.GetSeriesSummaryBySlug("weekly"); err != nil || summary.ID != "7" {
		t.Fatalf("slug summary = %+v, %v", summary, err)
	}
	if summary, err := c.GetSeriesSummaryByID("7"); err != nil || summary.ID != "7" {
		t.Fatalf("id summary = %+v, %v", summary, err)
	}
	if comments, err := c.GetComments(types.CommentParentMarket, 42, 10, 0); err != nil || len(comments) != 1 || comments[0].ID != "9" {
		t.Fatalf("comments = %+v, %v", comments, err)
	}
	if comments, err := c.GetComment(9, false); err != nil || len(comments) != 1 {
		t.Fatalf("comment = %+v, %v", comments, err)
	}
	if markets, err := c.GetAllMarkets(); err != nil || len(markets) != 1 {
		t.Fatalf("markets = %+v, %v", markets, err)
	}
}
