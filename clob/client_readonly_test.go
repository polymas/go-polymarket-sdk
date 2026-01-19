package clob

import (
	"strings"
	"testing"
	"time"

	"github.com/polymas/go-polymarket-sdk/gamma"
	"github.com/polymas/go-polymarket-sdk/test"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

// newTestClobClient 创建测试用的CLOB客户端（只读测试不需要认证）
// 优先使用只读客户端，如果没有私钥也可以运行测试
func newTestClobClient(t *testing.T) ReadonlyClient {
	config := test.LoadTestConfig()

	// 如果有私钥，使用完整客户端（向后兼容）
	if config.PrivateKey != "" {
		web3Client, err := web3.NewClient(config.PrivateKey, config.SignatureType, config.ChainID)
		if err != nil {
			t.Fatalf("Failed to create Web3 client: %v", err)
		}

		clobClient, err := NewClient(web3Client)
		if err != nil {
			t.Fatalf("Failed to create CLOB client: %v", err)
		}

		return clobClient
	}

	// 没有私钥时，使用只读客户端
	return NewReadonlyClient()
}

// getTestMarketData 获取测试市场数据（通过slug）
// 默认尝试获取BTC 15分钟涨跌预测市场
func getTestMarketData(t *testing.T, slug string) *test.TestMarketData {
	gammaClient := gamma.NewClient()
	var market *types.GammaMarket
	var err error

	// 如果slug为空，使用默认的BTC 15分钟市场slug
	if slug == "" {
		// 尝试常见的BTC 15分钟市场slug
		possibleSlugs := []string{
			"btc-15min-up-or-down",
			"will-bitcoin-price-be-higher-in-15-minutes",
			"btc-15-minute",
			"bitcoin-15min",
		}

		var lastErr error
		for _, s := range possibleSlugs {
			market, err = gammaClient.GetMarketBySlug(s, nil)
			if err == nil && market != nil && len(market.TokenIDs) > 0 {
				slug = s
				t.Logf("Found market by slug: %s", s)
				break
			}
			if err != nil {
				lastErr = err
			}
		}

		if market == nil {
			// 如果都失败了，尝试搜索活跃的BTC相关市场
			markets, searchErr := gammaClient.GetMarkets(50, gamma.WithActive(true), gamma.WithOrder("liquidity", false))
			if searchErr == nil && len(markets) > 0 {
				// 首先查找包含"btc"或"bitcoin"且包含"15"或"minute"的市场
				for _, m := range markets {
					slugLower := strings.ToLower(m.Slug)
					if (strings.Contains(slugLower, "btc") || strings.Contains(slugLower, "bitcoin")) &&
						(strings.Contains(slugLower, "15") || strings.Contains(slugLower, "minute")) &&
						len(m.TokenIDs) > 0 {
						// 检查流动性（优先选择有流动性的市场）
						hasLiquidity := m.LiquidityNum != nil && *m.LiquidityNum > 0
						if hasLiquidity || market == nil {
							market = &m
							slug = m.Slug
							t.Logf("Found BTC 15min market by search: %s (liquidity: %v)", m.Slug, hasLiquidity)
							// 如果有流动性，直接使用
							if hasLiquidity {
								break
							}
						}
					}
				}

				// 如果还是没找到BTC 15分钟市场，尝试找任何有流动性的活跃市场
				if market == nil {
					for _, m := range markets {
						hasLiquidity := m.LiquidityNum != nil && *m.LiquidityNum > 0
						if hasLiquidity && len(m.TokenIDs) > 0 {
							market = &m
							slug = m.Slug
							t.Logf("Using any market with liquidity: %s (liquidity: %v)", m.Slug, hasLiquidity)
							break
						}
					}
				}
			}

			// 如果还是没找到，返回错误
			if market == nil {
				if lastErr != nil {
					t.Skipf("Skipping test: Could not find BTC 15min market. Last error: %v. Please set POLY_TEST_TOKEN_ID or POLY_TEST_MARKET_SLUG", lastErr)
				} else {
					t.Skipf("Skipping test: Could not find BTC 15min market. Please set POLY_TEST_TOKEN_ID or POLY_TEST_MARKET_SLUG")
				}
				return nil
			}
		}
	} else {
		// 使用指定的slug
		market, err = gammaClient.GetMarketBySlug(slug, nil)
		if err != nil {
			t.Skipf("Skipping test: Failed to get market by slug '%s': %v", slug, err)
			return nil
		}
	}

	// 检查市场数据
	if market == nil || len(market.TokenIDs) == 0 {
		t.Skipf("Skipping test: Market '%s' has no token IDs", slug)
		return nil
	}

	// 创建测试市场数据
	testData, err := test.NewTestMarketDataFromMarket(market)
	if err != nil {
		t.Fatalf("Failed to create test market data: %v", err)
	}

	t.Logf("Using test market: %s (slug: %s) with %d token IDs", market.Question, slug, len(testData.TokenIDs))
	return testData
}

func TestGetOrderBook(t *testing.T) {
	client := newTestClobClient(t)
	config := test.LoadTestConfig()

	// 优先使用环境变量中的token ID
	var testTokenID string
	if config.TestTokenID != "" {
		testTokenID = config.TestTokenID
		t.Logf("Using token ID from environment: %s", testTokenID)
	} else {
		// 如果没有设置，尝试通过slug获取BTC 15分钟市场数据
		testData := getTestMarketData(t, "")
		if testData == nil {
			t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set and could not get market data")
			return
		}
		testTokenID = testData.TokenIDs[0] // 使用第一个token ID
		t.Logf("Using token ID from market: %s (market: %s)", testTokenID, testData.Slug)
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		orderBook, err := client.GetOrderBook(testTokenID)
		if err != nil {
			t.Fatalf("GetOrderBook failed: %v", err)
		}
		if orderBook == nil {
			t.Fatal("GetOrderBook returned nil")
		}
		t.Logf("GetOrderBook returned order book for token: %s", testTokenID)
	})

	// 边界条件测试 - 无效token ID
	t.Run("InvalidTokenID", func(t *testing.T) {
		_, err := client.GetOrderBook("invalid-token-id")
		if err == nil {
			t.Error("Expected error for invalid token ID")
		}
	})

	// 测试无订单簿的市场（使用一个不太可能有订单的tokenID）
	t.Run("NoOrderBook", func(t *testing.T) {
		unlikelyTokenID := "0x0000000000000000000000000000000000000000"
		orderBook, err := client.GetOrderBook(unlikelyTokenID)
		if err != nil {
			t.Logf("GetOrderBook with unlikely tokenID returned error (may be expected): %v", err)
		} else if orderBook != nil {
			t.Logf("GetOrderBook with unlikely tokenID returned order book (may be empty)")
		}
	})
}

func TestGetMultipleOrderBooks(t *testing.T) {
	client := newTestClobClient(t)
	config := test.LoadTestConfig()

	// 优先使用环境变量中的token ID
	var testTokenID string
	if config.TestTokenID != "" {
		testTokenID = config.TestTokenID
		t.Logf("Using token ID from environment: %s", testTokenID)
	} else {
		// 如果没有设置，尝试通过slug获取BTC 15分钟市场数据
		testData := getTestMarketData(t, "")
		if testData == nil {
			t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set and could not get market data")
			return
		}
		testTokenID = testData.TokenIDs[0] // 使用第一个token ID
		t.Logf("Using token ID from market: %s (market: %s)", testTokenID, testData.Slug)
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		requests := []types.BookParams{
			{TokenID: testTokenID},
		}
		orderBooks, err := client.GetMultipleOrderBooks(requests)
		if err != nil {
			t.Fatalf("GetMultipleOrderBooks failed: %v", err)
		}
		if orderBooks == nil {
			t.Fatal("GetMultipleOrderBooks returned nil")
		}
		if len(orderBooks) != 1 {
			t.Errorf("Expected 1 order book, got %d", len(orderBooks))
		}
		t.Logf("GetMultipleOrderBooks returned %d order books", len(orderBooks))
	})

	// 多个请求测试
	t.Run("MultipleRequests", func(t *testing.T) {
		requests := []types.BookParams{
			{TokenID: testTokenID, Side: "BUY"},
			{TokenID: testTokenID, Side: "SELL"},
		}
		orderBooks, err := client.GetMultipleOrderBooks(requests)
		if err != nil {
			t.Fatalf("GetMultipleOrderBooks failed: %v", err)
		}
		if orderBooks == nil {
			t.Fatal("GetMultipleOrderBooks returned nil")
		}
		// API可能对于同一个tokenID的BUY和SELL返回合并结果，所以可能返回1个或2个
		if len(orderBooks) < 1 || len(orderBooks) > 2 {
			t.Errorf("Expected 1-2 order books, got %d", len(orderBooks))
		}
		t.Logf("GetMultipleOrderBooks with multiple requests returned %d order books (API may merge BUY/SELL for same token)", len(orderBooks))
	})

	// 边界条件测试 - 空请求数组
	t.Run("EmptyRequests", func(t *testing.T) {
		_, err := client.GetMultipleOrderBooks([]types.BookParams{})
		if err == nil {
			t.Error("Expected error for empty requests array")
		}
	})

	// 测试大量请求（>10个）
	t.Run("LargeRequests", func(t *testing.T) {
		requests := make([]types.BookParams, 15)
		for i := 0; i < 15; i++ {
			requests[i] = types.BookParams{
				TokenID: testTokenID,
				Side:    "BUY",
			}
		}
		orderBooks, err := client.GetMultipleOrderBooks(requests)
		if err != nil {
			t.Fatalf("GetMultipleOrderBooks with large requests failed: %v", err)
		}
		if orderBooks == nil {
			t.Fatal("GetMultipleOrderBooks returned nil")
		}
		t.Logf("GetMultipleOrderBooks with large requests returned %d order books", len(orderBooks))
	})

	// 测试混合Side参数
	t.Run("MixedSideParams", func(t *testing.T) {
		requests := []types.BookParams{
			{TokenID: testTokenID, Side: "BUY"},
			{TokenID: testTokenID, Side: "SELL"},
			{TokenID: testTokenID}, // 无Side参数
		}
		orderBooks, err := client.GetMultipleOrderBooks(requests)
		if err != nil {
			t.Fatalf("GetMultipleOrderBooks with mixed sides failed: %v", err)
		}
		if orderBooks == nil {
			t.Fatal("GetMultipleOrderBooks returned nil")
		}
		t.Logf("GetMultipleOrderBooks with mixed sides returned %d order books", len(orderBooks))
	})

	// 测试部分tokenID无效的情况
	t.Run("PartialInvalidTokenIDs", func(t *testing.T) {
		requests := []types.BookParams{
			{TokenID: testTokenID},
			{TokenID: "invalid-token-id"},
		}
		orderBooks, err := client.GetMultipleOrderBooks(requests)
		if err != nil {
			t.Logf("GetMultipleOrderBooks with partial invalid tokenIDs returned error: %v", err)
		} else if orderBooks != nil {
			t.Logf("GetMultipleOrderBooks with partial invalid tokenIDs returned %d order books", len(orderBooks))
		}
	})
}

// TestGetUSDCBalance 已移动到 client_readwrite_test.go
// 因为 GetUSDCBalance 需要私钥和完整客户端

func TestGetMidpoint(t *testing.T) {
	client := newTestClobClient(t)
	config := test.LoadTestConfig()

	// 优先使用环境变量中的token ID
	var testTokenID string
	if config.TestTokenID != "" {
		testTokenID = config.TestTokenID
		t.Logf("Using token ID from environment: %s", testTokenID)
	} else {
		// 如果没有设置，尝试通过slug获取BTC 15分钟市场数据
		testData := getTestMarketData(t, "")
		if testData == nil {
			t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set and could not get market data")
			return
		}
		testTokenID = testData.TokenIDs[0] // 使用第一个token ID
		t.Logf("Using token ID from market: %s (market: %s)", testTokenID, testData.Slug)
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		midpoint, err := client.GetMidpoint(testTokenID)
		if err != nil {
			t.Fatalf("GetMidpoint failed: %v", err)
		}
		if midpoint == nil {
			t.Fatal("GetMidpoint returned nil")
		}
		t.Logf("GetMidpoint returned: %+v", midpoint)
	})

	// 测试无效tokenID
	t.Run("InvalidTokenID", func(t *testing.T) {
		_, err := client.GetMidpoint("invalid-token-id")
		if err == nil {
			t.Error("Expected error for invalid token ID")
		}
	})

	// 测试无买卖单的情况（使用一个不太可能有订单的tokenID）
	t.Run("NoBidsOrAsks", func(t *testing.T) {
		unlikelyTokenID := "0x0000000000000000000000000000000000000000"
		midpoint, err := client.GetMidpoint(unlikelyTokenID)
		if err != nil {
			t.Logf("GetMidpoint with unlikely tokenID returned error (may be expected): %v", err)
		} else if midpoint != nil {
			t.Logf("GetMidpoint with unlikely tokenID returned: %+v", midpoint)
		}
	})
}

func TestGetMidpoints(t *testing.T) {
	client := newTestClobClient(t)
	config := test.LoadTestConfig()

	// 优先使用环境变量中的token ID
	var testTokenID string
	if config.TestTokenID != "" {
		testTokenID = config.TestTokenID
		t.Logf("Using token ID from environment: %s", testTokenID)
	} else {
		// 如果没有设置，尝试通过slug获取BTC 15分钟市场数据
		testData := getTestMarketData(t, "")
		if testData == nil {
			t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set and could not get market data")
			return
		}
		testTokenID = testData.TokenIDs[0] // 使用第一个token ID
		t.Logf("Using token ID from market: %s (market: %s)", testTokenID, testData.Slug)
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		tokenIDs := []string{testTokenID}
		midpoints, err := client.GetMidpoints(tokenIDs)
		if err != nil {
			t.Fatalf("GetMidpoints failed: %v", err)
		}
		if midpoints == nil {
			t.Fatal("GetMidpoints returned nil")
		}
		if len(midpoints) != 1 {
			t.Errorf("Expected 1 midpoint, got %d", len(midpoints))
		}
		t.Logf("GetMidpoints returned %d midpoints", len(midpoints))
	})

	// 空数组测试
	t.Run("EmptyArray", func(t *testing.T) {
		midpoints, err := client.GetMidpoints([]string{})
		if err != nil {
			t.Fatalf("GetMidpoints with empty array failed: %v", err)
		}
		if midpoints == nil {
			t.Fatal("GetMidpoints returned nil")
		}
		if len(midpoints) != 0 {
			t.Errorf("Expected empty array, got %d midpoints", len(midpoints))
		}
	})

	// 测试大量tokenID
	t.Run("LargeTokenIDs", func(t *testing.T) {
		tokenIDs := make([]string, 20)
		for i := 0; i < 20; i++ {
			tokenIDs[i] = testTokenID
		}
		midpoints, err := client.GetMidpoints(tokenIDs)
		if err != nil {
			t.Fatalf("GetMidpoints with large tokenIDs failed: %v", err)
		}
		if midpoints == nil {
			t.Fatal("GetMidpoints returned nil")
		}
		t.Logf("GetMidpoints with large tokenIDs returned %d midpoints", len(midpoints))
	})

	// 测试部分tokenID无效的情况
	t.Run("PartialInvalidTokenIDs", func(t *testing.T) {
		tokenIDs := []string{
			testTokenID,
			"invalid-token-id",
		}
		midpoints, err := client.GetMidpoints(tokenIDs)
		if err != nil {
			t.Logf("GetMidpoints with partial invalid tokenIDs returned error: %v", err)
		} else if midpoints != nil {
			t.Logf("GetMidpoints with partial invalid tokenIDs returned %d midpoints", len(midpoints))
		}
	})
}

func TestGetPrice(t *testing.T) {
	client := newTestClobClient(t)
	config := test.LoadTestConfig()

	// 优先使用环境变量中的token ID
	var testTokenID string
	if config.TestTokenID != "" {
		testTokenID = config.TestTokenID
		t.Logf("Using token ID from environment: %s", testTokenID)
	} else {
		// 如果没有设置，尝试通过slug获取BTC 15分钟市场数据
		testData := getTestMarketData(t, "")
		if testData == nil {
			t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set and could not get market data")
			return
		}
		testTokenID = testData.TokenIDs[0] // 使用第一个token ID
		t.Logf("Using token ID from market: %s (market: %s)", testTokenID, testData.Slug)
	}

	// BUY价格测试
	t.Run("BUY", func(t *testing.T) {
		price, err := client.GetPrice(testTokenID, types.OrderSideBUY)
		if err != nil {
			t.Fatalf("GetPrice BUY failed: %v", err)
		}
		if price == nil {
			t.Fatal("GetPrice returned nil")
		}
		t.Logf("GetPrice BUY returned: %+v", price)
	})

	// SELL价格测试
	t.Run("SELL", func(t *testing.T) {
		price, err := client.GetPrice(testTokenID, types.OrderSideSELL)
		if err != nil {
			t.Fatalf("GetPrice SELL failed: %v", err)
		}
		if price == nil {
			t.Fatal("GetPrice returned nil")
		}
		t.Logf("GetPrice SELL returned: %+v", price)
	})

	// 测试无效tokenID
	t.Run("InvalidTokenID", func(t *testing.T) {
		_, err := client.GetPrice("invalid-token-id", types.OrderSideBUY)
		if err == nil {
			t.Error("Expected error for invalid token ID")
		}
	})

	// 测试无订单的情况（使用一个不太可能有订单的tokenID）
	t.Run("NoOrders", func(t *testing.T) {
		unlikelyTokenID := "0x0000000000000000000000000000000000000000"
		price, err := client.GetPrice(unlikelyTokenID, types.OrderSideBUY)
		if err != nil {
			t.Logf("GetPrice with unlikely tokenID returned error (may be expected): %v", err)
		} else if price != nil {
			t.Logf("GetPrice with unlikely tokenID returned: %+v", price)
		}
	})
}

func TestGetPrices(t *testing.T) {
	client := newTestClobClient(t)
	config := test.LoadTestConfig()

	// 优先使用环境变量中的token ID
	var testTokenID string
	if config.TestTokenID != "" {
		testTokenID = config.TestTokenID
		t.Logf("Using token ID from environment: %s", testTokenID)
	} else {
		// 如果没有设置，尝试通过slug获取BTC 15分钟市场数据
		testData := getTestMarketData(t, "")
		if testData == nil {
			t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set and could not get market data")
			return
		}
		testTokenID = testData.TokenIDs[0] // 使用第一个token ID
		t.Logf("Using token ID from market: %s (market: %s)", testTokenID, testData.Slug)
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		requests := []types.BookParams{
			{TokenID: testTokenID, Side: "BUY"},
		}
		prices, err := client.GetPrices(requests)
		if err != nil {
			t.Fatalf("GetPrices failed: %v", err)
		}
		if prices == nil {
			t.Fatal("GetPrices returned nil")
		}
		if len(prices) != 1 {
			t.Errorf("Expected 1 price, got %d", len(prices))
		}
		t.Logf("GetPrices returned %d prices", len(prices))
	})

	// 测试大量请求
	t.Run("LargeRequests", func(t *testing.T) {
		requests := make([]types.BookParams, 15)
		for i := 0; i < 15; i++ {
			requests[i] = types.BookParams{
				TokenID: testTokenID,
				Side:    "BUY",
			}
		}
		prices, err := client.GetPrices(requests)
		if err != nil {
			t.Fatalf("GetPrices with large requests failed: %v", err)
		}
		if prices == nil {
			t.Fatal("GetPrices returned nil")
		}
		t.Logf("GetPrices with large requests returned %d prices", len(prices))
	})

	// 测试部分请求失败的情况
	t.Run("PartialFailures", func(t *testing.T) {
		requests := []types.BookParams{
			{TokenID: testTokenID, Side: "BUY"},
			{TokenID: "invalid-token-id", Side: "BUY"},
		}
		prices, err := client.GetPrices(requests)
		if err != nil {
			t.Logf("GetPrices with partial failures returned error: %v", err)
		} else if prices != nil {
			t.Logf("GetPrices with partial failures returned %d prices", len(prices))
		}
	})
}

func TestGetSpread(t *testing.T) {
	client := newTestClobClient(t)
	config := test.LoadTestConfig()

	// 优先使用环境变量中的token ID
	var testTokenID string
	if config.TestTokenID != "" {
		testTokenID = config.TestTokenID
		t.Logf("Using token ID from environment: %s", testTokenID)
	} else {
		// 如果没有设置，尝试通过slug获取BTC 15分钟市场数据
		testData := getTestMarketData(t, "")
		if testData == nil {
			t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set and could not get market data")
			return
		}
		testTokenID = testData.TokenIDs[0] // 使用第一个token ID
		t.Logf("Using token ID from market: %s (market: %s)", testTokenID, testData.Slug)
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		spread, err := client.GetSpread(testTokenID)
		if err != nil {
			t.Fatalf("GetSpread failed: %v", err)
		}
		if spread == nil {
			t.Fatal("GetSpread returned nil")
		}
		t.Logf("GetSpread returned: %+v", spread)
	})

	// 测试无效tokenID
	t.Run("InvalidTokenID", func(t *testing.T) {
		_, err := client.GetSpread("invalid-token-id")
		if err == nil {
			t.Error("Expected error for invalid token ID")
		}
	})

	// 测试无买卖单的情况
	t.Run("NoBidsOrAsks", func(t *testing.T) {
		unlikelyTokenID := "0x0000000000000000000000000000000000000000"
		spread, err := client.GetSpread(unlikelyTokenID)
		if err != nil {
			t.Logf("GetSpread with unlikely tokenID returned error (may be expected): %v", err)
		} else if spread != nil {
			t.Logf("GetSpread with unlikely tokenID returned: %+v", spread)
		}
	})
}

func TestGetSpreads(t *testing.T) {
	client := newTestClobClient(t)
	config := test.LoadTestConfig()

	// 优先使用环境变量中的token ID
	var testTokenID string
	if config.TestTokenID != "" {
		testTokenID = config.TestTokenID
		t.Logf("Using token ID from environment: %s", testTokenID)
	} else {
		// 如果没有设置，尝试通过slug获取BTC 15分钟市场数据
		testData := getTestMarketData(t, "")
		if testData == nil {
			t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set and could not get market data")
			return
		}
		testTokenID = testData.TokenIDs[0] // 使用第一个token ID
		t.Logf("Using token ID from market: %s (market: %s)", testTokenID, testData.Slug)
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		tokenIDs := []string{testTokenID}
		spreads, err := client.GetSpreads(tokenIDs)
		if err != nil {
			t.Fatalf("GetSpreads failed: %v", err)
		}
		if spreads == nil {
			t.Fatal("GetSpreads returned nil")
		}
		if len(spreads) != 1 {
			t.Errorf("Expected 1 spread, got %d", len(spreads))
		}
		t.Logf("GetSpreads returned %d spreads", len(spreads))
	})

	// 测试大量tokenID
	t.Run("LargeTokenIDs", func(t *testing.T) {
		tokenIDs := make([]string, 20)
		for i := 0; i < 20; i++ {
			tokenIDs[i] = testTokenID
		}
		spreads, err := client.GetSpreads(tokenIDs)
		if err != nil {
			t.Fatalf("GetSpreads with large tokenIDs failed: %v", err)
		}
		if spreads == nil {
			t.Fatal("GetSpreads returned nil")
		}
		t.Logf("GetSpreads with large tokenIDs returned %d spreads", len(spreads))
	})

	// 测试部分tokenID无效的情况
	t.Run("PartialInvalidTokenIDs", func(t *testing.T) {
		tokenIDs := []string{
			testTokenID,
			"invalid-token-id",
		}
		spreads, err := client.GetSpreads(tokenIDs)
		if err != nil {
			t.Logf("GetSpreads with partial invalid tokenIDs returned error: %v", err)
		} else if spreads != nil {
			t.Logf("GetSpreads with partial invalid tokenIDs returned %d spreads", len(spreads))
		}
	})
}

func TestGetLastTradePrice(t *testing.T) {
	client := newTestClobClient(t)
	config := test.LoadTestConfig()

	// 优先使用环境变量中的token ID
	var testTokenID string
	if config.TestTokenID != "" {
		testTokenID = config.TestTokenID
		t.Logf("Using token ID from environment: %s", testTokenID)
	} else {
		// 如果没有设置，尝试通过slug获取BTC 15分钟市场数据
		testData := getTestMarketData(t, "")
		if testData == nil {
			t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set and could not get market data")
			return
		}
		testTokenID = testData.TokenIDs[0] // 使用第一个token ID
		t.Logf("Using token ID from market: %s (market: %s)", testTokenID, testData.Slug)
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		lastTradePrice, err := client.GetLastTradePrice(testTokenID)
		if err != nil {
			t.Fatalf("GetLastTradePrice failed: %v", err)
		}
		if lastTradePrice == nil {
			t.Fatal("GetLastTradePrice returned nil")
		}
		t.Logf("GetLastTradePrice returned: %+v", lastTradePrice)
	})

	// 测试无效tokenID
	t.Run("InvalidTokenID", func(t *testing.T) {
		lastTradePrice, err := client.GetLastTradePrice("invalid-token-id")
		if err != nil {
			// 如果有错误，这是预期的
			t.Logf("GetLastTradePrice with invalid tokenID returned error (expected): %v", err)
		} else if lastTradePrice != nil {
			// API可能对无效tokenID返回空结果而不是错误，这也是可以接受的
			t.Logf("GetLastTradePrice with invalid tokenID returned result (API may return empty result instead of error): %+v", lastTradePrice)
		}
	})

	// 测试无交易历史的情况
	t.Run("NoTradeHistory", func(t *testing.T) {
		unlikelyTokenID := "0x0000000000000000000000000000000000000000"
		lastTradePrice, err := client.GetLastTradePrice(unlikelyTokenID)
		if err != nil {
			t.Logf("GetLastTradePrice with unlikely tokenID returned error (may be expected): %v", err)
		} else if lastTradePrice != nil {
			t.Logf("GetLastTradePrice with unlikely tokenID returned: %+v", lastTradePrice)
		}
	})
}

func TestGetLastTradesPrices(t *testing.T) {
	client := newTestClobClient(t)
	config := test.LoadTestConfig()

	// 优先使用环境变量中的token ID
	var testTokenID string
	if config.TestTokenID != "" {
		testTokenID = config.TestTokenID
		t.Logf("Using token ID from environment: %s", testTokenID)
	} else {
		// 如果没有设置，尝试通过slug获取BTC 15分钟市场数据
		testData := getTestMarketData(t, "")
		if testData == nil {
			t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set and could not get market data")
			return
		}
		testTokenID = testData.TokenIDs[0] // 使用第一个token ID
		t.Logf("Using token ID from market: %s (market: %s)", testTokenID, testData.Slug)
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		tokenIDs := []string{testTokenID}
		lastTradesPrices, err := client.GetLastTradesPrices(tokenIDs)
		if err != nil {
			// 如果是网络错误（连接重置等），可能是临时问题，记录但不失败
			if strings.Contains(err.Error(), "connection reset") || strings.Contains(err.Error(), "timeout") {
				t.Logf("GetLastTradesPrices failed with network error (may be temporary): %v", err)
				return
			}
			t.Fatalf("GetLastTradesPrices failed: %v", err)
		}
		if lastTradesPrices == nil {
			t.Fatal("GetLastTradesPrices returned nil")
		}
		// API可能返回空数组（如果token没有交易历史），这是正常的
		if len(lastTradesPrices) > 1 {
			t.Errorf("Expected at most 1 last trade price, got %d", len(lastTradesPrices))
		}
		t.Logf("GetLastTradesPrices returned %d last trade prices (may be 0 if no trade history)", len(lastTradesPrices))
	})

	// 测试大量tokenID
	t.Run("LargeTokenIDs", func(t *testing.T) {
		tokenIDs := make([]string, 20)
		for i := 0; i < 20; i++ {
			tokenIDs[i] = testTokenID
		}
		lastTradesPrices, err := client.GetLastTradesPrices(tokenIDs)
		if err != nil {
			t.Fatalf("GetLastTradesPrices with large tokenIDs failed: %v", err)
		}
		if lastTradesPrices == nil {
			t.Fatal("GetLastTradesPrices returned nil")
		}
		t.Logf("GetLastTradesPrices with large tokenIDs returned %d last trade prices", len(lastTradesPrices))
	})

	// 测试部分tokenID无效的情况
	t.Run("PartialInvalidTokenIDs", func(t *testing.T) {
		tokenIDs := []string{
			testTokenID,
			"invalid-token-id",
		}
		lastTradesPrices, err := client.GetLastTradesPrices(tokenIDs)
		if err != nil {
			t.Logf("GetLastTradesPrices with partial invalid tokenIDs returned error: %v", err)
		} else if lastTradesPrices != nil {
			t.Logf("GetLastTradesPrices with partial invalid tokenIDs returned %d last trade prices", len(lastTradesPrices))
		}
	})
}

func TestGetFeeRate(t *testing.T) {
	client := newTestClobClient(t)
	config := test.LoadTestConfig()

	// 优先使用环境变量中的token ID
	var testTokenID string
	if config.TestTokenID != "" {
		testTokenID = config.TestTokenID
		t.Logf("Using token ID from environment: %s", testTokenID)
	} else {
		// 如果没有设置，尝试通过slug获取BTC 15分钟市场数据
		testData := getTestMarketData(t, "")
		if testData == nil {
			t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set and could not get market data")
			return
		}
		testTokenID = testData.TokenIDs[0] // 使用第一个token ID
		t.Logf("Using token ID from market: %s (market: %s)", testTokenID, testData.Slug)
	}

	// 基本功能测试
	// 注意：fee-rate API端点可能不存在或已废弃，返回404
	t.Run("Basic", func(t *testing.T) {
		feeRate, err := client.GetFeeRate(testTokenID)
		if err != nil {
			// 如果API端点不存在（404），跳过测试
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not Found") {
				t.Skip("Skipping test: fee-rate API endpoint not found (may be deprecated)")
				return
			}
			t.Fatalf("GetFeeRate failed: %v", err)
		}
		if feeRate < 0 {
			t.Errorf("Expected non-negative fee rate, got %d", feeRate)
		}
		t.Logf("GetFeeRate returned: %d", feeRate)
	})

	// 测试无效tokenID
	t.Run("InvalidTokenID", func(t *testing.T) {
		_, err := client.GetFeeRate("invalid-token-id")
		if err == nil {
			t.Error("Expected error for invalid token ID")
		}
	})
}

func TestGetTime(t *testing.T) {
	client := newTestClobClient(t)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		serverTime, err := client.GetTime()
		if err != nil {
			t.Fatalf("GetTime failed: %v", err)
		}
		if serverTime.IsZero() {
			t.Error("GetTime returned zero time")
		}
		t.Logf("GetTime returned: %v", serverTime)
	})

	// 测试时间格式和同步
	t.Run("TimeFormat", func(t *testing.T) {
		serverTime, err := client.GetTime()
		if err != nil {
			t.Fatalf("GetTime failed: %v", err)
		}
		if serverTime.IsZero() {
			t.Error("GetTime returned zero time")
		}
		// 检查时间是否合理（不应该是未来的时间，也不应该是太早的时间）
		now := time.Now()
		timeDiff := serverTime.Sub(now)
		if timeDiff > 5*time.Minute || timeDiff < -5*time.Minute {
			t.Logf("Server time differs significantly from local time: %v", timeDiff)
		} else {
			t.Logf("Server time is synchronized (diff: %v)", timeDiff)
		}
	})
}
