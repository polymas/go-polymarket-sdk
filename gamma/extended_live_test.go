package gamma

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestGammaExtendedReadOnlyLive(t *testing.T) {
	if os.Getenv("POLYMARKET_LIVE_TEST") != "1" {
		t.Skip("set POLYMARKET_LIVE_TEST=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c := NewClient()
	status, err := c.GetStatusContext(ctx)
	if err != nil || status != "OK" {
		t.Fatalf("status=%q err=%v", status, err)
	}
	teams, err := c.GetTeamsContext(ctx, 1, 0)
	if err != nil || len(teams) == 0 {
		t.Fatalf("teams=%d err=%v", len(teams), err)
	}
	events, err := c.GetEventsPageContext(ctx, 1, EventsKeysetOptions{})
	if err != nil || len(events.Events) == 0 {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	markets, err := c.GetMarketsPageContext(ctx, 1, WithLocale("en"))
	if err != nil || len(markets.Markets) == 0 {
		t.Fatalf("markets=%+v err=%v", markets, err)
	}
	marketTypes, err := c.GetSportsMarketTypesContext(ctx)
	if err != nil || len(marketTypes.MarketTypes) == 0 {
		t.Fatalf("marketTypes=%+v err=%v", marketTypes, err)
	}
}
