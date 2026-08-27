package clob

import (
	"context"
	"math"
	"os"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/polymas/go-polymarket-sdk/gamma"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

// TestBatchOrderLatencyLive 测量生产 CLOB 批量下单的端到端延迟。
//
// 安全约束：
//   - 默认跳过，只有 POLY_RUN_ORDER_LATENCY_BENCHMARK=1 才执行；
//   - 选择不同 condition 的活跃市场，在最低 tick 以 PostOnly BUY 挂单；
//   - 每笔 maker notional 不超过 0.05 USDC，且下单前确认 best ask > tick；
//   - 每批返回后立即按确切 order ID 撤单，绝不调用会影响其他订单的 CancelAll；
//   - 任一订单出现 matched 状态即停止测试并报错。
func TestBatchOrderLatencyLive(t *testing.T) {
	if os.Getenv("POLY_RUN_ORDER_LATENCY_BENCHMARK") != "1" {
		t.Skip("set POLY_RUN_ORDER_LATENCY_BENCHMARK=1 to enable the live batch latency benchmark")
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

	const (
		candidateCount = 5
		samplesPerMode = 20
		maxNotional    = 0.05
	)
	candidates := selectLatencyCandidates(t, client, candidateCount, maxNotional)
	if len(candidates) < 2 {
		t.Fatalf("found only %d safe candidates; need at least 2 for a batch test", len(candidates))
	}
	for _, candidate := range candidates {
		t.Logf(
			"candidate slug=%s condition=%s token=%s tick=%s size=%g bestAsk=%g notional=%g",
			candidate.slug, candidate.conditionID, candidate.tokenID, candidate.tickSize,
			candidate.size, candidate.bestAsk, candidate.price*candidate.size,
		)
	}

	outstanding := make(map[types.Keccak256]struct{})
	defer func() {
		ids := orderIDSetValues(outstanding)
		if len(ids) == 0 {
			return
		}
		if _, cleanupErr := client.CancelOrders(ids); cleanupErr != nil {
			t.Errorf("final cleanup failed for %d order(s): %v", len(ids), cleanupErr)
		}
	}()

	runMode := func(name string, post func([]types.OrderArgs, []types.OrderType) ([]types.OrderPostResponse, error)) []time.Duration {
		durations := make([]time.Duration, 0, samplesPerMode)
		postDurations := make([]time.Duration, 0, samplesPerMode)
		reconciliationDurations := make([]time.Duration, 0, samplesPerMode)
		settlementDurations := make([]time.Duration, 0, samplesPerMode)
		statuses := make(map[string]int)
		for sample := 0; sample < samplesPerMode; sample++ {
			args, orderTypes := latencyBatch(candidates)
			start := time.Now()
			responses, postErr := post(args, orderTypes)
			elapsed := time.Since(start)

			ids, unsafeResponse := batchCleanupOrderIDs(responses, outstanding)
			if len(ids) > 0 {
				cancelResponse, cancelErr := client.CancelOrders(ids)
				if cancelErr != nil {
					t.Fatalf("%s sample %d cancel %d order(s): %v", name, sample+1, len(ids), cancelErr)
				}
				for _, id := range cancelResponse.Canceled {
					delete(outstanding, id)
				}
				if len(cancelResponse.Canceled) != len(ids) {
					t.Fatalf("%s sample %d incomplete cancel: %+v", name, sample+1, cancelResponse)
				}
			}
			if unsafeResponse != "" {
				t.Fatalf("SAFETY STOP after cleanup: %s sample %d %s", name, sample+1, unsafeResponse)
			}
			if postErr != nil {
				t.Fatalf("%s sample %d post failed after cleanup: %v; responses=%+v", name, sample+1, postErr, responses)
			}
			if len(responses) != len(args) {
				t.Fatalf("%s sample %d response count=%d want=%d", name, sample+1, len(responses), len(args))
			}
			var postDuration, reconciliationDuration, settlementDuration time.Duration
			for _, response := range responses {
				statuses[response.Status]++
				postDuration = maxDuration(postDuration, response.Timing.PostDuration)
				reconciliationDuration = maxDuration(reconciliationDuration, response.Timing.ReconciliationWaitDuration)
				settlementDuration = maxDuration(settlementDuration, response.Timing.SettlementWaitDuration)
			}
			durations = append(durations, elapsed)
			postDurations = append(postDurations, postDuration)
			reconciliationDurations = append(reconciliationDurations, reconciliationDuration)
			settlementDurations = append(settlementDurations, settlementDuration)
			time.Sleep(50 * time.Millisecond)
		}
		p50, p95, p99 := latencyPercentiles(durations)
		_, postP95, _ := latencyPercentiles(postDurations)
		_, reconciliationP95, _ := latencyPercentiles(reconciliationDurations)
		_, settlementP95, _ := latencyPercentiles(settlementDurations)
		t.Logf(
			"%s batch=%d samples=%d p50=%v p95=%v p99=%v post_p95=%v reconciliation_p95=%v settlement_p95=%v statuses=%v",
			name, len(candidates), len(durations), p50, p95, p99,
			postP95, reconciliationP95, settlementP95, statuses,
		)
		return durations
	}

	instant := runMode("instant", client.CreateAndPostOrdersInstant)
	andWait := runMode("and_wait", func(args []types.OrderArgs, orderTypes []types.OrderType) ([]types.OrderPostResponse, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return client.CreateAndPostOrdersAndWait(ctx, args, orderTypes)
	})
	_, instantP95, _ := latencyPercentiles(instant)
	_, waitP95, _ := latencyPercentiles(andWait)
	t.Logf("batch latency summary: orders=%d instant_p95=%v and_wait_p95=%v wait_overhead_p95=%v", len(candidates), instantP95, waitP95, waitP95-instantP95)
}

type latencyCandidate struct {
	slug        string
	conditionID types.Keccak256
	tokenID     string
	tickSize    types.TickSize
	price       float64
	size        float64
	bestAsk     float64
	negRisk     bool
}

func selectLatencyCandidates(t *testing.T, client Client, limit int, maxNotional float64) []latencyCandidate {
	t.Helper()
	markets, err := gamma.NewClient().GetMarkets(
		300,
		gamma.WithActive(true),
		gamma.WithClosed(false),
		gamma.WithOrder("startDate", false),
	)
	if err != nil {
		t.Fatalf("list active markets: %v", err)
	}

	selected := make([]latencyCandidate, 0, limit)
	seenConditions := make(map[types.Keccak256]struct{}, limit)
	for _, market := range markets {
		if len(selected) >= limit {
			break
		}
		if !market.AcceptingOrders || !market.EnableOrderBook || market.ConditionID == "" || len(market.TokenIDs) == 0 {
			continue
		}
		if _, exists := seenConditions[market.ConditionID]; exists {
			continue
		}
		size := 5.0
		if market.OrderMinSize != nil && *market.OrderMinSize > size {
			size = *market.OrderMinSize
		}

		for _, tokenID := range market.TokenIDs {
			tickSize, tickErr := client.GetTickSize(tokenID)
			if tickErr != nil {
				continue
			}
			price, parseErr := parseRequiredTickSize(tickSize)
			if parseErr != nil || price*size > maxNotional+1e-12 {
				continue
			}
			book, bookErr := client.GetOrderBook(tokenID)
			if bookErr != nil || book == nil || len(book.Asks) == 0 {
				continue
			}
			bestAsk := math.Inf(1)
			for _, ask := range book.Asks {
				askPrice := float64(ask.Price)
				if askPrice > 0 && askPrice < bestAsk {
					bestAsk = askPrice
				}
			}
			if math.IsInf(bestAsk, 1) || bestAsk <= price {
				continue
			}
			selected = append(selected, latencyCandidate{
				slug: market.Slug, conditionID: market.ConditionID, tokenID: tokenID,
				tickSize: tickSize, price: price, size: size, bestAsk: bestAsk, negRisk: market.NegRisk,
			})
			seenConditions[market.ConditionID] = struct{}{}
			break
		}
	}
	return selected
}

func latencyBatch(candidates []latencyCandidate) ([]types.OrderArgs, []types.OrderType) {
	args := make([]types.OrderArgs, len(candidates))
	orderTypes := make([]types.OrderType, len(candidates))
	for i := range candidates {
		candidate := &candidates[i]
		args[i] = types.OrderArgs{
			TokenID: candidate.tokenID, Price: candidate.price, Size: candidate.size,
			Side: types.OrderSideBUY, TickSize: candidate.tickSize, PostOnly: true,
			NegRisk: &candidate.negRisk,
		}
		orderTypes[i] = types.OrderTypeGTC
	}
	return args, orderTypes
}

func batchCleanupOrderIDs(
	responses []types.OrderPostResponse,
	outstanding map[types.Keccak256]struct{},
) ([]types.Keccak256, string) {
	ids := make([]types.Keccak256, 0, len(responses))
	unsafeResponse := ""
	for i, response := range responses {
		cleanupID := response.OrderID
		if cleanupID == "" && response.IsUnknown() {
			cleanupID = response.ExpectedOrderID
		}
		if cleanupID != "" {
			ids = append(ids, cleanupID)
			outstanding[cleanupID] = struct{}{}
		}
		if unsafeResponse == "" && (response.Status == "matched" || len(response.TradeIDs) > 0 || len(response.TransactionsHashes) > 0) {
			unsafeResponse = "response[" + strconv.Itoa(i) + "] may have matched"
		}
	}
	return ids, unsafeResponse
}

func orderIDSetValues(values map[types.Keccak256]struct{}) []types.Keccak256 {
	result := make([]types.Keccak256, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	return result
}

func latencyPercentiles(values []time.Duration) (time.Duration, time.Duration, time.Duration) {
	if len(values) == 0 {
		return 0, 0, 0
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	percentile := func(p float64) time.Duration {
		index := int(math.Ceil(p*float64(len(sorted)))) - 1
		if index < 0 {
			index = 0
		}
		return sorted[index]
	}
	return percentile(0.50), percentile(0.95), percentile(0.99)
}

func maxDuration(a, b time.Duration) time.Duration {
	if b > a {
		return b
	}
	return a
}

func TestLatencyPercentilesUsesNearestRank(t *testing.T) {
	values := make([]time.Duration, 20)
	for i := range values {
		values[i] = time.Duration(20-i) * time.Millisecond
	}
	p50, p95, p99 := latencyPercentiles(values)
	if p50 != 10*time.Millisecond || p95 != 19*time.Millisecond || p99 != 20*time.Millisecond {
		t.Fatalf("p50=%v p95=%v p99=%v", p50, p95, p99)
	}
}
