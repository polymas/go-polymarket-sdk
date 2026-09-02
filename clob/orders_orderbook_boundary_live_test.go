package clob

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/polymas/go-polymarket-sdk/gamma"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

// TestOrderbookFailuresMixedBatchLive sends one production /orders request with:
// two real tokens from closed Gamma markets, two valid uint256 values that do
// not belong to any known orderbook, and one live token. The live order is
// PostOnly at the minimum tick with maker notional capped at 0.05 USDC.
func TestOrderbookFailuresMixedBatchLive(t *testing.T) {
	if os.Getenv("POLY_RUN_ORDERBOOK_BOUNDARY_INTEGRATION") != "1" {
		t.Skip("set POLY_RUN_ORDERBOOK_BOUNDARY_INTEGRATION=1 to enable the live mixed-orderbook test")
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
	gammaClient := gamma.NewClient()

	type selectedToken struct {
		tokenID string
		slug    string
		negRisk bool
	}

	closedMarkets, err := gammaClient.GetMarkets(
		100,
		gamma.WithClosed(true),
		gamma.WithOrder("closedTime", false),
	)
	if err != nil {
		t.Fatalf("list closed markets: %v", err)
	}
	closed := make([]selectedToken, 0, 2)
	seenClosed := make(map[string]struct{})
	for _, market := range closedMarkets {
		for _, tokenID := range market.TokenIDs {
			if _, exists := seenClosed[tokenID]; exists {
				continue
			}
			_, bookErr := client.GetOrderBook(tokenID)
			if bookErr == nil || !strings.Contains(strings.ToLower(bookErr.Error()), "orderbook") {
				continue
			}
			seenClosed[tokenID] = struct{}{}
			closed = append(closed, selectedToken{tokenID: tokenID, slug: market.Slug, negRisk: market.NegRisk})
			break
		}
		if len(closed) == 2 {
			break
		}
	}
	if len(closed) != 2 {
		t.Fatalf("found %d confirmed closed orderbooks, want 2", len(closed))
	}

	activeMarkets, err := gammaClient.GetMarkets(
		100,
		gamma.WithActive(true),
		gamma.WithClosed(false),
		gamma.WithOrder("startDate", false),
	)
	if err != nil {
		t.Fatalf("list active markets: %v", err)
	}

	const maxMakerNotional = 0.05
	type activeCandidate struct {
		tokenID  string
		slug     string
		tickSize types.TickSize
		size     float64
		negRisk  bool
	}
	var active *activeCandidate
	for _, market := range activeMarkets {
		if !market.AcceptingOrders || !market.EnableOrderBook {
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
			tick, tickErr := parseRequiredTickSize(tickSize)
			if tickErr != nil || tick*size > maxMakerNotional+1e-12 {
				continue
			}
			book, bookErr := client.GetOrderBook(tokenID)
			if bookErr != nil || book == nil || len(book.Asks) == 0 {
				continue
			}
			bestAsk := 2.0
			for _, ask := range book.Asks {
				price, conversionErr := ask.Price.Float64()
				if conversionErr != nil {
					continue
				}
				if price > 0 && price < bestAsk {
					bestAsk = price
				}
			}
			if bestAsk <= tick {
				continue
			}
			active = &activeCandidate{
				tokenID: tokenID, slug: market.Slug, tickSize: tickSize,
				size: size, negRisk: market.NegRisk,
			}
			break
		}
		if active != nil {
			break
		}
	}
	if active == nil {
		t.Fatal("no active market satisfies the 0.05 USDC maker-notional safety limit")
	}

	before, err := client.GetOrders(nil, nil, &active.tokenID)
	if err != nil {
		t.Fatalf("list live-token orders before test: %v", err)
	}
	beforeIDs := make(map[types.Keccak256]struct{}, len(before))
	for _, order := range before {
		beforeIDs[order.OrderID] = struct{}{}
	}

	cleanupNewOrders := func() {
		after, listErr := client.GetOrders(nil, nil, &active.tokenID)
		if listErr != nil {
			t.Logf("cleanup: list live-token orders: %v", listErr)
			return
		}
		for _, order := range after {
			if _, existed := beforeIDs[order.OrderID]; existed {
				continue
			}
			if _, cancelErr := client.CancelOrder(order.OrderID); cancelErr != nil {
				t.Logf("cleanup: cancel unexpected order %s: %v", order.OrderID, cancelErr)
			} else {
				t.Logf("cleanup: canceled unexpected live order %s", order.OrderID)
			}
		}
	}
	defer cleanupNewOrders()

	const (
		invalidToken1 = "115792089237316195423570985008687907853269984665640564039457584007913129639935"
		invalidToken2 = "115792089237316195423570985008687907853269984665640564039457584007913129639934"
	)
	activeTick, err := parseRequiredTickSize(active.tickSize)
	if err != nil {
		t.Fatal(err)
	}
	closedTick := types.TickSize0_01
	closedTickPrice, err := parseRequiredTickSize(closedTick)
	if err != nil {
		t.Fatal(err)
	}
	falseValue := false

	orders := []types.OrderArgs{
		{TokenID: closed[0].tokenID, Price: closedTickPrice, Size: 5, Side: types.OrderSideBUY, TickSize: closedTick, PostOnly: true, NegRisk: &closed[0].negRisk},
		{TokenID: invalidToken1, Price: closedTickPrice, Size: 5, Side: types.OrderSideBUY, TickSize: closedTick, PostOnly: true, NegRisk: &falseValue},
		{TokenID: active.tokenID, Price: activeTick, Size: active.size, Side: types.OrderSideBUY, TickSize: active.tickSize, PostOnly: true, NegRisk: &active.negRisk},
		{TokenID: closed[1].tokenID, Price: closedTickPrice, Size: 5, Side: types.OrderSideBUY, TickSize: closedTick, PostOnly: true, NegRisk: &closed[1].negRisk},
		{TokenID: invalidToken2, Price: closedTickPrice, Size: 5, Side: types.OrderSideBUY, TickSize: closedTick, PostOnly: true, NegRisk: &falseValue},
	}
	orderTypes := make([]types.OrderType, len(orders))
	for i := range orderTypes {
		orderTypes[i] = types.OrderTypeGTC
	}

	t.Logf("batch[0] closed slug=%s token=%s", closed[0].slug, closed[0].tokenID)
	t.Logf("batch[1] invalid token=%s", invalidToken1)
	t.Logf("batch[2] active slug=%s token=%s tick=%s size=%g notional=%g", active.slug, active.tokenID, active.tickSize, active.size, activeTick*active.size)
	t.Logf("batch[3] closed slug=%s token=%s", closed[1].slug, closed[1].tokenID)
	t.Logf("batch[4] invalid token=%s", invalidToken2)

	results, postErr := client.CreateAndPostOrders(orders, orderTypes)
	t.Logf("CreateAndPostOrders error: %v", postErr)
	if postErr == nil || !strings.Contains(strings.ToLower(postErr.Error()), "invalid token id") {
		t.Fatalf("mixed request error=%v, want top-level invalid token id", postErr)
	}
	if len(results) != len(orders) {
		t.Fatalf("result count=%d, want %d; error=%v", len(results), len(orders), postErr)
	}
	for i, result := range results {
		t.Logf("result[%d] token=%s status=%q orderID=%q errorMsg=%q", i, orders[i].TokenID, result.Status, result.OrderID, result.ErrorMsg)
		if result.Status != OrderNotSubmittedStatus {
			t.Errorf("result[%d] status=%q, want %q", i, result.Status, OrderNotSubmittedStatus)
		}
	}

	after, err := client.GetOrders(nil, nil, &active.tokenID)
	if err != nil {
		t.Fatalf("list live-token orders after test: %v", err)
	}
	newOrderIDs := make([]types.Keccak256, 0)
	for _, order := range after {
		if _, existed := beforeIDs[order.OrderID]; !existed {
			newOrderIDs = append(newOrderIDs, order.OrderID)
		}
	}
	t.Logf("normal token new open orders after mixed request: %v", newOrderIDs)

	if len(newOrderIDs) > 0 {
		t.Errorf("mixed request unexpectedly created %d live order(s): %v", len(newOrderIDs), newOrderIDs)
	}

	t.Logf("summary: closed=%d invalid=%d active=%d total=%d", len(closed), 2, 1, len(orders))
}
