package clob

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

// TestMarketOrderRoundTripLive 用同一 token 先买 5 shares 再卖回，用于验证
// BUY Amount、SELL Shares、FOK、保护价和结算等待的生产端到端行为。
// 默认跳过；运行前应选择 tick=0.01、买卖各至少 5 shares 深度的紧价差市场。
func TestMarketOrderRoundTripLive(t *testing.T) {
	if os.Getenv("POLY_RUN_MARKET_ORDER_INTEGRATION") != "1" {
		t.Skip("set POLY_RUN_MARKET_ORDER_INTEGRATION=1 to enable the live market-order round trip")
	}
	tokenID := os.Getenv("POLY_MARKET_ORDER_TEST_TOKEN")
	if tokenID == "" {
		t.Fatal("POLY_MARKET_ORDER_TEST_TOKEN is required")
	}
	privateKey := os.Getenv("POLY_PRIVATE_KEY")
	if privateKey == "" {
		privateKey = os.Getenv("poly_sec")
	}
	if privateKey == "" {
		t.Fatal("POLY_PRIVATE_KEY or poly_sec is required")
	}
	signatureType, err := strconv.Atoi(defaultEnv("POLY_SIGNATURE_TYPE", "2"))
	if err != nil {
		t.Fatalf("parse POLY_SIGNATURE_TYPE: %v", err)
	}

	w3, err := web3.NewClient(privateKey, types.SignatureType(signatureType), types.Polygon)
	if err != nil {
		t.Fatalf("create web3 client: %v", err)
	}
	defer w3.Close()
	client, err := NewClient(w3)
	if err != nil {
		t.Fatalf("create CLOB client: %v", err)
	}

	book, err := client.GetOrderBook(tokenID)
	if err != nil {
		t.Fatalf("get initial book: %v", err)
	}
	if book.TickSize != types.TickSize0_01 {
		t.Fatalf("safety check: tick_size=%s, want 0.01", book.TickSize)
	}
	bestBid, bestAsk, bidSize, askSize := liveBestPrices(t, book)
	if bidSize < 5 || askSize < 5 || bestAsk <= bestBid || (bestAsk-bestBid)*5 > 0.25+1e-12 {
		t.Fatalf("safety check failed: bid=%g x %g ask=%g x %g round-trip spread cost=%g", bestBid, bidSize, bestAsk, askSize, (bestAsk-bestBid)*5)
	}
	buyAmount := math.Round(bestAsk*5*100) / 100
	estimatedBuy, err := client.CalculateMarketPrice(tokenID, types.OrderSideBUY, buyAmount, types.OrderTypeFOK)
	if err != nil || estimatedBuy != bestAsk {
		t.Fatalf("CalculateMarketPrice BUY = %g, %v; want %g", estimatedBuy, err, bestAsk)
	}
	estimatedSell, err := client.CalculateMarketPrice(tokenID, types.OrderSideSELL, 5, types.OrderTypeFOK)
	if err != nil || estimatedSell != bestBid {
		t.Fatalf("CalculateMarketPrice SELL = %g, %v; want %g", estimatedSell, err, bestBid)
	}
	t.Logf("read-only estimate: BUY amount=%g price=%g; SELL shares=5 price=%g", buyAmount, estimatedBuy, estimatedSell)

	negRisk := book.NegRisk
	bought := os.Getenv("POLY_MARKET_ORDER_SELL_ONLY") == "1"
	sold := false
	defer func() {
		if !bought || sold {
			return
		}
		if _, cleanupErr := sellFiveSharesWithRetry(t, client, w3, tokenID, &negRisk, 30*time.Second); cleanupErr != nil {
			t.Errorf("cleanup SELL failed: %v", cleanupErr)
		}
	}()

	if !bought {
		buyCtx, buyCancel := context.WithTimeout(context.Background(), 20*time.Second)
		buy, buyErr := client.CreateAndPostMarketOrderAndWait(buyCtx, types.MarketOrderArgs{
			TokenID: tokenID, Side: types.OrderSideBUY, Amount: buyAmount, MaxPrice: bestAsk,
			TickSize: book.TickSize, OrderType: types.OrderTypeFOK, NegRisk: &negRisk,
		})
		buyCancel()
		if buyErr != nil || buy == nil || !buy.Accepted() || buy.Status != "matched" {
			t.Fatalf("BUY failed: response=%+v err=%v", buy, buyErr)
		}
		bought = true
		t.Logf("BUY matched: amount=%g maxPrice=%g response=%+v", buyAmount, bestAsk, buy)
	} else {
		t.Log("sell-only mode: using the 5 shares left by a previous BUY")
	}

	sell, err := sellFiveSharesWithRetry(t, client, w3, tokenID, &negRisk, 30*time.Second)
	if err != nil {
		t.Fatalf("SELL failed: %v", err)
	}
	sold = true
	t.Logf("SELL matched: shares=5 response=%+v", sell)
}

// TestMarketOrderTakerPrecisionLive 验证生产 CLOB 对 tick=0.001 市价单
// taker amount 允许 5 位、拒绝 6 位的实际约束。两笔都使用远低于 best ask 的 FAK BUY，
// 只验证请求校验与无可成交深度结果，不会成交。
func TestMarketOrderTakerPrecisionLive(t *testing.T) {
	if os.Getenv("POLY_RUN_MARKET_PRECISION_INTEGRATION") != "1" {
		t.Skip("set POLY_RUN_MARKET_PRECISION_INTEGRATION=1 to enable the live precision probe")
	}
	tokenID := os.Getenv("POLY_MARKET_ORDER_TEST_TOKEN")
	privateKey := os.Getenv("POLY_PRIVATE_KEY")
	if privateKey == "" {
		privateKey = os.Getenv("poly_sec")
	}
	if tokenID == "" || privateKey == "" {
		t.Fatal("POLY_MARKET_ORDER_TEST_TOKEN and private key are required")
	}
	signatureType, err := strconv.Atoi(defaultEnv("POLY_SIGNATURE_TYPE", "2"))
	if err != nil {
		t.Fatal(err)
	}
	w3, err := web3.NewClient(privateKey, types.SignatureType(signatureType), types.Polygon)
	if err != nil {
		t.Fatal(err)
	}
	defer w3.Close()
	publicClient, err := NewClient(w3)
	if err != nil {
		t.Fatal(err)
	}
	client := publicClient.(*polymarketClobClient)
	book, err := client.GetOrderBook(tokenID)
	if err != nil {
		t.Fatal(err)
	}
	if book.TickSize != types.TickSize0_001 {
		t.Fatalf("tick_size=%s, want 0.001", book.TickSize)
	}
	_, bestAsk, _, _ := liveBestPrices(t, book)
	const testPrice = 0.003
	if bestAsk <= testPrice {
		t.Fatalf("safety check: best ask %g must be above test price %g", bestAsk, testPrice)
	}
	negRisk := book.NegRisk
	orderArgs := []types.OrderArgs{{
		TokenID: tokenID, Price: testPrice, Size: 5, Side: types.OrderSideBUY,
		TickSize: book.TickSize, NegRisk: &negRisk,
	}}
	bad, badErr := client.orderClientImpl.postOrdersBatchV2OnceWithAmounts(
		orderArgs,
		[]types.OrderType{types.OrderTypeFAK},
		false,
		[]orderAmountOverride{{maker: big.NewInt(5_000_000), taker: big.NewInt(1_666_666_666)}},
		false,
	)
	if badErr != nil {
		t.Fatalf("six-decimal probe returned request-level error: %v", badErr)
	}
	if len(bad) != 1 || !strings.Contains(strings.ToLower(bad[0].ErrorMsg), "invalid amounts") {
		t.Fatalf("six-decimal response=%+v, want invalid amounts", bad)
	}
	t.Logf("six-decimal taker rejected as expected: %s", bad[0].ErrorMsg)

	good, goodErr := client.CreateAndPostMarketOrderInstant(types.MarketOrderArgs{
		TokenID: tokenID, Side: types.OrderSideBUY, Amount: 5, MaxPrice: testPrice,
		TickSize: book.TickSize, OrderType: types.OrderTypeFAK, NegRisk: &negRisk,
	})
	if goodErr != nil {
		t.Fatalf("five-decimal market order request failed: %v", goodErr)
	}
	if good == nil || good.Accepted() ||
		strings.Contains(strings.ToLower(good.ErrorMsg), "invalid amounts") ||
		!strings.Contains(strings.ToLower(good.ErrorMsg), "no orders found") ||
		len(good.TransactionsHashes) != 0 || len(good.TradeIDs) != 0 {
		t.Fatalf("five-decimal response=%+v, want non-matching but precision-valid FAK", good)
	}
	t.Logf("five-decimal taker passed amount validation and reached matching: %s", good.ErrorMsg)
}

func sellFiveSharesWithRetry(
	t *testing.T,
	client Client,
	w3 web3.Client,
	tokenID string,
	negRisk *bool,
	timeout time.Duration,
) (*types.OrderPostResponse, error) {
	t.Helper()
	proxy, err := w3.GetPolyProxyAddress()
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	var lastResponse *types.OrderPostResponse
	var lastErr error
	for time.Now().Before(deadline) {
		balance, balanceErr := w3.GetTokenBalance(tokenID, proxy)
		if balanceErr != nil || balance+1e-9 < 5 {
			lastErr = balanceErr
			time.Sleep(time.Second)
			continue
		}
		book, bookErr := client.GetOrderBook(tokenID)
		if bookErr != nil {
			lastErr = bookErr
			time.Sleep(time.Second)
			continue
		}
		bestBid, _, bidSize, _ := liveBestPrices(t, book)
		if bidSize < 5 {
			lastErr = fmt.Errorf("best bid only has %g shares", bidSize)
			time.Sleep(time.Second)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		lastResponse, lastErr = client.CreateAndPostMarketOrderAndWait(ctx, types.MarketOrderArgs{
			TokenID: tokenID, Side: types.OrderSideSELL, Shares: 5, MinPrice: bestBid,
			TickSize: book.TickSize, OrderType: types.OrderTypeFOK, NegRisk: negRisk,
		})
		cancel()
		if lastErr == nil && lastResponse != nil && lastResponse.Accepted() && lastResponse.Status == "matched" {
			return lastResponse, nil
		}
		t.Logf("SELL not ready yet: chainBalance=%g response=%+v err=%v", balance, lastResponse, lastErr)
		time.Sleep(time.Second)
	}
	return lastResponse, fmt.Errorf("SELL did not match within %v: response=%+v err=%v", timeout, lastResponse, lastErr)
}

func liveBestPrices(t *testing.T, book *types.OrderBookSummary) (bestBid, bestAsk, bidSize, askSize float64) {
	t.Helper()
	bestBid = -1
	bestAsk = 2
	for _, level := range book.Bids {
		price, size, err := parseOrderLevel(level)
		if err != nil {
			t.Fatalf("parse bid: %v", err)
		}
		if price > bestBid {
			bestBid, bidSize = price, size
		}
	}
	for _, level := range book.Asks {
		price, size, err := parseOrderLevel(level)
		if err != nil {
			t.Fatalf("parse ask: %v", err)
		}
		if price < bestAsk {
			bestAsk, askSize = price, size
		}
	}
	if bestBid < 0 || bestAsk > 1 {
		t.Fatalf("book has no two-sided liquidity: bids=%d asks=%d", len(book.Bids), len(book.Asks))
	}
	return bestBid, bestAsk, bidSize, askSize
}
