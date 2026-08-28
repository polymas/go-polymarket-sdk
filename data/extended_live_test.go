package data

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/polymas/go-polymarket-sdk/types"
)

func TestDataExtendedReadOnlyLive(t *testing.T) {
	if os.Getenv("POLYMARKET_LIVE_TEST") != "1" {
		t.Skip("set POLYMARKET_LIVE_TEST=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := NewClient()
	user := types.EthAddress("0xc3acf5878a03523d09a3ac859943445d7baeb964")
	conditionID := "0xa467b14d51f01b957109d9cbb1d6c124fab2a089d52ed8f471d23c2812e743b7"

	health, err := c.GetHealthContext(ctx)
	if err != nil || health.Data != "OK" {
		t.Fatalf("health=%+v err=%v", health, err)
	}
	snapshot, err := c.GetAccountingSnapshotContext(ctx, user)
	if err != nil || len(snapshot) < 4 || !bytes.Equal(snapshot[:4], []byte{'P', 'K', 3, 4}) {
		t.Fatalf("snapshot bytes=%d err=%v", len(snapshot), err)
	}
	if _, err := c.GetComboActivityContext(ctx, user, ComboActivityOptions{Limit: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetComboPositionsContext(ctx, user, ComboPositionsOptions{Limit: 1}); err != nil {
		t.Fatal(err)
	}
	openPositions, err := c.GetPositionsContext(ctx, user, WithPositionsIncludeArchived(true))
	if err != nil || len(openPositions) == 0 || openPositions[0].GrossInitialValue <= 0 {
		t.Fatalf("positions=%+v err=%v", openPositions, err)
	}
	traded, err := c.GetTradedMarketsContext(ctx, user)
	if err != nil || traded.Traded <= 0 {
		t.Fatalf("traded=%+v err=%v", traded, err)
	}
	oi, err := c.GetOpenInterestContext(ctx, []string{conditionID})
	if err != nil || len(oi) == 0 || oi[0].Value <= 0 {
		t.Fatalf("oi=%+v err=%v", oi, err)
	}
	holders, err := c.GetHoldersContext(ctx, []string{conditionID}, 1, 1)
	if err != nil || len(holders) == 0 || len(holders[0].Holders) == 0 {
		t.Fatalf("holders=%+v err=%v", holders, err)
	}
	positions, err := c.GetMarketPositionsContext(ctx, types.Keccak256(conditionID), MarketPositionsOptions{Limit: 1})
	if err != nil || len(positions) == 0 || len(positions[0].Positions) == 0 {
		t.Fatalf("market positions=%+v err=%v", positions, err)
	}
	closed, err := c.GetClosedPositionsContext(ctx, user, ClosedPositionsOptions{Limit: 1})
	if err != nil || len(closed) == 0 {
		t.Fatalf("closed=%+v err=%v", closed, err)
	}
	if volume, err := c.GetLiveVolumeContext(ctx, 2890); err != nil || volume == nil {
		t.Fatalf("live volume=%+v err=%v", volume, err)
	}
	if other, err := c.GetOtherPositionsContext(ctx, 2890, user); err != nil || other == nil {
		t.Fatalf("other=%+v err=%v", other, err)
	}
	if builders, err := c.GetBuilderLeaderboardContext(ctx, BuilderLeaderboardOptions{Limit: 1}); err != nil || len(builders) == 0 {
		t.Fatalf("builders=%+v err=%v", builders, err)
	}
	if volume, err := c.GetBuilderVolumeContext(ctx, LeaderboardDay); err != nil || len(volume) == 0 {
		t.Fatalf("builder volume=%+v err=%v", volume, err)
	}
	if traders, err := c.GetTraderLeaderboardContext(ctx, TraderLeaderboardOptions{Limit: 1}); err != nil || len(traders) == 0 {
		t.Fatalf("traders=%+v err=%v", traders, err)
	}
}
