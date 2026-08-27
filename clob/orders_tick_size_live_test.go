package clob

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/polymas/go-polymarket-sdk/gamma"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

// TestExplicitTickSizeLivePostAndCancel 验证业务层显式获取并传入 tick size 后，
// 订单可以在生产 CLOB 成功挂出和撤销。
//
// 安全约束：
//   - 默认跳过，只有 POLY_RUN_TICK_SIZE_INTEGRATION=1 才执行；
//   - 只下 PostOnly GTC，价格固定为市场最小 tick，不允许吃单；
//   - maker notional 上限为 0.05 USDC；
//   - 挂单成功后立即按订单 ID 撤销，失败路径也用 defer 再尝试撤销。
func TestExplicitTickSizeLivePostAndCancel(t *testing.T) {
	if os.Getenv("POLY_RUN_TICK_SIZE_INTEGRATION") != "1" {
		t.Skip("set POLY_RUN_TICK_SIZE_INTEGRATION=1 to enable the live post/cancel contract test")
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

	markets, err := gamma.NewClient().GetMarkets(
		100,
		gamma.WithActive(true),
		gamma.WithClosed(false),
		gamma.WithOrder("startDate", false),
	)
	if err != nil {
		t.Fatalf("list active markets: %v", err)
	}

	const maxMakerNotional = 0.05
	type candidate struct {
		market   types.GammaMarket
		tokenID  string
		tickSize types.TickSize
		size     float64
		bestAsk  float64
	}
	var selected *candidate

	for _, market := range markets {
		if !market.AcceptingOrders || !market.EnableOrderBook || len(market.TokenIDs) == 0 {
			continue
		}
		size := 5.0
		if market.OrderMinSize != nil && *market.OrderMinSize > size {
			size = *market.OrderMinSize
		}

		for _, tokenID := range market.TokenIDs {
			tickSize, err := client.GetTickSize(tokenID)
			if err != nil {
				continue
			}
			tick, err := parseRequiredTickSize(tickSize)
			if err != nil || tick*size > maxMakerNotional+1e-12 {
				continue
			}

			book, err := client.GetOrderBook(tokenID)
			if err != nil || book == nil || len(book.Asks) == 0 {
				continue
			}
			bestAsk := 2.0
			for _, ask := range book.Asks {
				price := float64(ask.Price)
				if price > 0 && price < bestAsk {
					bestAsk = price
				}
			}
			if bestAsk <= tick {
				continue
			}

			selected = &candidate{
				market:   market,
				tokenID:  tokenID,
				tickSize: tickSize,
				size:     size,
				bestAsk:  bestAsk,
			}
			break
		}
		if selected != nil {
			break
		}
	}
	if selected == nil {
		t.Fatal("no active market satisfies the 0.05 USDC maker-notional safety limit")
	}

	tick, err := parseRequiredTickSize(selected.tickSize)
	if err != nil {
		t.Fatal(err)
	}
	negRisk := selected.market.NegRisk
	t.Logf(
		"safe candidate: slug=%s token=%s tick=%s size=%g bestAsk=%g maxNotional=%g",
		selected.market.Slug,
		selected.tokenID,
		selected.tickSize,
		selected.size,
		selected.bestAsk,
		tick*selected.size,
	)

	response, err := client.PostOrder(types.OrderArgs{
		TokenID:  selected.tokenID,
		Price:    tick,
		Size:     selected.size,
		Side:     types.OrderSideBUY,
		TickSize: selected.tickSize,
		PostOnly: true,
		NegRisk:  &negRisk,
	}, types.OrderTypeGTC)
	if err != nil {
		t.Fatalf("post explicit-tick order: %v", err)
	}
	if response == nil || response.ErrorMsg != "" || response.OrderID == "" {
		t.Fatalf("order was not accepted: %+v", response)
	}
	if response.ExpectedOrderID != response.OrderID {
		t.Fatalf("local expected order hash %s does not match CLOB order ID %s", response.ExpectedOrderID, response.OrderID)
	}
	var queried *types.OpenOrder
	queryDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(queryDeadline) {
		queried, err = client.GetOrder(response.OrderID)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("query live order %s: %v", response.OrderID, err)
	}
	if queried.OrderID != response.OrderID {
		t.Fatalf("queried order ID = %s, want %s", queried.OrderID, response.OrderID)
	}

	canceled := false
	defer func() {
		if canceled {
			return
		}
		if _, cancelErr := client.CancelOrder(response.OrderID); cancelErr != nil {
			t.Logf("cleanup cancel failed for order %s: %v", response.OrderID, cancelErr)
		}
	}()

	cancelResponse, err := client.CancelOrder(response.OrderID)
	if err != nil {
		t.Fatalf("cancel order %s: %v", response.OrderID, err)
	}
	for _, orderID := range cancelResponse.Canceled {
		if orderID == response.OrderID {
			canceled = true
			break
		}
	}
	if !canceled {
		t.Fatalf("order %s not confirmed canceled: %+v", response.OrderID, cancelResponse)
	}
	t.Logf("live explicit-tick order %s posted and canceled; timing=%+v", response.OrderID, response.Timing)
}

func defaultEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
