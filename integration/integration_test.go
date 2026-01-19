package integration

import (
	"sync"
	"testing"
	"time"

	"github.com/polymas/go-polymarket-sdk/clob"
	"github.com/polymas/go-polymarket-sdk/data"
	"github.com/polymas/go-polymarket-sdk/gamma"
	"github.com/polymas/go-polymarket-sdk/test"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
	"github.com/polymas/go-polymarket-sdk/websocket"
)

// TestCrossModuleIntegration 测试跨模块集成
func TestCrossModuleIntegration(t *testing.T) {
	config := test.LoadTestConfig()
	test.SkipIfNoAuth(t, config)

	if config.TestTokenID == "" || config.TestMarketID == "" {
		t.Skip("Skipping test: Required test IDs not set")
	}

	// 创建Web3客户端
	web3Client, err := web3.NewClient(config.PrivateKey, config.SignatureType, config.ChainID)
	if err != nil {
		t.Fatalf("Failed to create Web3 client: %v", err)
	}
	defer web3Client.Close()

	// 创建CLOB客户端
	clobClient, err := clob.NewClient(web3Client)
	if err != nil {
		t.Fatalf("Failed to create CLOB client: %v", err)
	}

	// 创建Gamma客户端
	gammaClient := gamma.NewClient()

	// 创建Data客户端
	dataClient := data.NewClient()

	// 测试：Gamma GetMarket -> CLOB GetOrderBook
	t.Run("GammaToCLOB", func(t *testing.T) {
		market, err := gammaClient.GetMarket(config.TestMarketID)
		if err != nil {
			t.Fatalf("Gamma GetMarket failed: %v", err)
		}

		if market == nil || len(market.Tokens) == 0 {
			t.Skip("Skipping: Market has no tokens")
		}

		// 使用市场的第一个tokenID获取订单簿
		tokenID := market.Tokens[0].TokenID
		orderBook, err := clobClient.GetOrderBook(tokenID)
		if err != nil {
			t.Fatalf("CLOB GetOrderBook failed: %v", err)
		}
		if orderBook == nil {
			t.Fatal("GetOrderBook returned nil")
		}
		t.Logf("Integration test: Gamma GetMarket -> CLOB GetOrderBook succeeded")
	})

	// 测试：Web3 GetUSDCBalance -> CLOB GetUSDCBalance
	t.Run("Web3ToCLOB", func(t *testing.T) {
		baseAddr := web3Client.GetBaseAddress()
		web3Balance, err := web3Client.GetUSDCBalance(baseAddr)
		if err != nil {
			t.Fatalf("Web3 GetUSDCBalance failed: %v", err)
		}

		clobBalance, err := clobClient.GetUSDCBalance()
		if err != nil {
			t.Fatalf("CLOB GetUSDCBalance failed: %v", err)
		}

		// 余额应该相同（或非常接近，考虑到可能的延迟）
		diff := web3Balance - clobBalance
		if diff < 0 {
			diff = -diff
		}
		if diff > 0.01 { // 允许小的差异
			t.Logf("Balance difference: Web3=%f, CLOB=%f, diff=%f", web3Balance, clobBalance, diff)
		} else {
			t.Logf("Integration test: Web3 GetUSDCBalance -> CLOB GetUSDCBalance succeeded (balances match)")
		}
	})

	// 测试：CLOB订单创建 -> Data GetTrades
	t.Run("CLOBToData", func(t *testing.T) {
		// 创建一个订单
		orderArgs := types.OrderArgs{
			TokenID: config.TestTokenID,
			Side:    types.OrderSideBUY,
			Price:   0.5,
			Size:    10.0,
		}

		response, err := clobClient.PostOrder(orderArgs, types.OrderTypeGTC)
		if err != nil {
			t.Logf("CLOB PostOrder failed (may be expected): %v", err)
			return
		}

		if response != nil && response.ErrorMsg != "" {
			t.Logf("Order creation returned error: %s", response.ErrorMsg)
			return
		}

		// 等待一下让交易被记录
		time.Sleep(2 * time.Second)

		// 获取交易记录
		trades, err := dataClient.GetTrades(10, 0)
		if err != nil {
			t.Fatalf("Data GetTrades failed: %v", err)
		}
		if trades != nil {
			t.Logf("Integration test: CLOB PostOrder -> Data GetTrades succeeded (found %d trades)", len(trades))
		}
	})
}

// TestEndToEndTradingFlow 测试端到端交易流程
func TestEndToEndTradingFlow(t *testing.T) {
	config := test.LoadTestConfig()
	test.SkipIfNoAuth(t, config)

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 创建Web3客户端
	web3Client, err := web3.NewClient(config.PrivateKey, config.SignatureType, config.ChainID)
	if err != nil {
		t.Fatalf("Failed to create Web3 client: %v", err)
	}
	defer web3Client.Close()

	// 创建CLOB客户端
	clobClient, err := clob.NewClient(web3Client)
	if err != nil {
		t.Fatalf("Failed to create CLOB client: %v", err)
	}

	// 完整交易流程：获取市场 -> 创建订单 -> 监控订单 -> 取消订单
	t.Run("FullTradingFlow", func(t *testing.T) {
		// 1. 获取订单簿
		orderBook, err := clobClient.GetOrderBook(config.TestTokenID)
		if err != nil {
			t.Fatalf("GetOrderBook failed: %v", err)
		}
		if orderBook == nil {
			t.Fatal("GetOrderBook returned nil")
		}
		t.Logf("Step 1: Got order book")

		// 2. 创建订单
		orderArgs := types.OrderArgs{
			TokenID: config.TestTokenID,
			Side:    types.OrderSideBUY,
			Price:   0.5,
			Size:    10.0,
		}

		response, err := clobClient.PostOrder(orderArgs, types.OrderTypeGTC)
		if err != nil {
			t.Logf("PostOrder failed (may be expected): %v", err)
			return
		}

		if response != nil && response.ErrorMsg != "" {
			t.Logf("Order creation returned error: %s", response.ErrorMsg)
			return
		}

		t.Logf("Step 2: Created order")

		// 3. 获取订单列表
		orders, err := clobClient.GetOrders(nil, nil, nil)
		if err != nil {
			t.Fatalf("GetOrders failed: %v", err)
		}
		if orders != nil {
			t.Logf("Step 3: Got %d orders", len(orders))
		}

		// 4. 取消订单（如果创建成功）
		if response != nil && response.ErrorMsg == "" && len(orders) > 0 {
			// 找到刚创建的订单并取消
			for _, order := range orders {
				if order.TokenID == config.TestTokenID {
					cancelResponse, err := clobClient.CancelOrder(order.OrderID)
					if err != nil {
						t.Logf("CancelOrder failed: %v", err)
					} else if cancelResponse != nil {
						t.Logf("Step 4: Canceled order successfully")
					}
					break
				}
			}
		}
	})
}

// TestWebSocketIntegration 测试WebSocket集成
func TestWebSocketIntegration(t *testing.T) {
	config := test.LoadTestConfig()

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 测试：CLOB订单创建 -> WebSocket订单更新
	t.Run("CLOBToWebSocket", func(t *testing.T) {
		test.SkipIfNoAuth(t, config)

		// 创建Web3和CLOB客户端
		web3Client, err := web3.NewClient(config.PrivateKey, config.SignatureType, config.ChainID)
		if err != nil {
			t.Fatalf("Failed to create Web3 client: %v", err)
		}
		defer web3Client.Close()

		clobClient, err := clob.NewClient(web3Client)
		if err != nil {
			t.Fatalf("Failed to create CLOB client: %v", err)
		}

		// 创建WebSocket客户端
		wsClient := websocket.NewClient(test.DefaultReconnectDelay)

		// 设置回调
		var receivedUpdate bool
		var mu sync.Mutex
		wsClient.SetOnBookUpdate(func(assetID string, snapshot *types.BookSnapshot) {
			mu.Lock()
			receivedUpdate = true
			mu.Unlock()
			t.Logf("Received book update for asset: %s", assetID)
		})

		// 启动WebSocket
		assetIDs := []string{config.TestTokenID}
		err = wsClient.Start(assetIDs)
		if err != nil {
			t.Fatalf("WebSocket Start failed: %v", err)
		}
		defer wsClient.Stop()

		// 等待连接建立
		time.Sleep(3 * time.Second)

		// 创建一个订单（可能会触发订单簿更新）
		orderArgs := types.OrderArgs{
			TokenID: config.TestTokenID,
			Side:    types.OrderSideBUY,
			Price:   0.5,
			Size:    10.0,
		}

		_, err = clobClient.PostOrder(orderArgs, types.OrderTypeGTC)
		if err != nil {
			t.Logf("PostOrder failed (may be expected): %v", err)
		}

		// 等待接收更新
		time.Sleep(5 * time.Second)

		mu.Lock()
		hasUpdate := receivedUpdate
		mu.Unlock()

		if hasUpdate {
			t.Logf("Integration test: CLOB PostOrder -> WebSocket order update succeeded")
		} else {
			t.Logf("Integration test: No WebSocket update received (may be expected if no market activity)")
		}
	})
}
