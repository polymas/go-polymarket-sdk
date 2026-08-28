package clob

import (
	"os"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

// TestGetTradesLiveReadOnly 验证生产 CLOB 的 L2 鉴权、分页响应和成交模型。
// 只读测试，默认跳过；不会发送订单。
func TestGetTradesLiveReadOnly(t *testing.T) {
	if os.Getenv("POLY_RUN_TRADES_INTEGRATION") != "1" {
		t.Skip("set POLY_RUN_TRADES_INTEGRATION=1 to enable the live trades contract test")
	}
	privateKey := os.Getenv("POLY_PRIVATE_KEY")
	if privateKey == "" {
		privateKey = os.Getenv("poly_sec")
	}
	if privateKey == "" {
		t.Fatal("POLY_PRIVATE_KEY or poly_sec is required")
	}
	signatureTypeValue, err := strconv.Atoi(defaultEnv("POLY_SIGNATURE_TYPE", "0"))
	if err != nil {
		t.Fatalf("parse POLY_SIGNATURE_TYPE: %v", err)
	}
	w3, err := web3.NewClient(privateKey, types.SignatureType(signatureTypeValue), types.Polygon)
	if err != nil {
		t.Fatalf("create web3 client: %v", err)
	}
	defer w3.Close()
	client, err := NewClient(w3)
	if err != nil {
		t.Fatalf("create CLOB client: %v", err)
	}

	queryStart := time.Now()
	trades, err := client.GetTrades(&types.ClobTradeParams{After: time.Now().Add(-24 * time.Hour).Unix()})
	if err != nil {
		t.Fatalf("GetTrades: %v", err)
	}
	queryDuration := time.Since(queryStart)
	serverLifecycleDelays := make([]time.Duration, 0, len(trades))
	for i, trade := range trades {
		if trade.ID == "" || trade.Market == (types.ConditionID{}) || trade.TokenID == (types.TokenID{}) {
			t.Fatalf("trade[%d] missing required IDs: %+v", i, trade)
		}
		matchedAt, matchErr := strconv.ParseInt(trade.MatchTime, 10, 64)
		updatedAt, updateErr := strconv.ParseInt(trade.LastUpdate, 10, 64)
		if matchErr == nil && updateErr == nil && updatedAt >= matchedAt {
			serverLifecycleDelays = append(serverLifecycleDelays, time.Duration(updatedAt-matchedAt)*time.Second)
		}
	}
	sort.Slice(serverLifecycleDelays, func(i, j int) bool {
		return serverLifecycleDelays[i] < serverLifecycleDelays[j]
	})
	if len(serverLifecycleDelays) > 0 {
		p50 := serverLifecycleDelays[(len(serverLifecycleDelays)-1)*50/100]
		p95 := serverLifecycleDelays[(len(serverLifecycleDelays)-1)*95/100]
		t.Logf(
			"decoded %d authenticated CLOB trade(s) in %v; historical match->last_update p50=%v p95=%v",
			len(trades), queryDuration, p50, p95,
		)
		return
	}
	t.Logf("decoded %d authenticated CLOB trade(s) in %v; no comparable lifecycle timestamps", len(trades), queryDuration)
}
