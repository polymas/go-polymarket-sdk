package clob

import (
	"testing"

	"github.com/polymas/go-polymarket-sdk/test"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

// newTestClobClient 创建测试用的CLOB客户端（只读测试不需要认证）
func newTestClobClient(t *testing.T) Client {
	config := test.LoadTestConfig()
	if config.PrivateKey == "" {
		// 对于只读测试，如果没有私钥，尝试创建一个只读客户端
		// 但CLOB客户端需要Web3Client，所以我们需要一个最小配置
		t.Skip("Skipping test: POLY_PRIVATE_KEY not set (required for CLOB client)")
	}

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

func TestGetOrderBook(t *testing.T) {
	client := newTestClobClient(t)
	config := test.LoadTestConfig()

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		orderBook, err := client.GetOrderBook(config.TestTokenID)
		if err != nil {
			t.Fatalf("GetOrderBook failed: %v", err)
		}
		if orderBook == nil {
			t.Fatal("GetOrderBook returned nil")
		}
		t.Logf("GetOrderBook returned order book for token: %s", config.TestTokenID)
	})

	// 边界条件测试 - 无效token ID
	t.Run("InvalidTokenID", func(t *testing.T) {
		_, err := client.GetOrderBook("invalid-token-id")
		if err == nil {
			t.Error("Expected error for invalid token ID")
		}
	})
}

func TestGetMultipleOrderBooks(t *testing.T) {
	client := newTestClobClient(t)
	config := test.LoadTestConfig()

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		requests := []types.BookParams{
			{TokenID: config.TestTokenID},
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
			{TokenID: config.TestTokenID, Side: "BUY"},
			{TokenID: config.TestTokenID, Side: "SELL"},
		}
		orderBooks, err := client.GetMultipleOrderBooks(requests)
		if err != nil {
			t.Fatalf("GetMultipleOrderBooks failed: %v", err)
		}
		if orderBooks == nil {
			t.Fatal("GetMultipleOrderBooks returned nil")
		}
		if len(orderBooks) != 2 {
			t.Errorf("Expected 2 order books, got %d", len(orderBooks))
		}
	})

	// 边界条件测试 - 空请求数组
	t.Run("EmptyRequests", func(t *testing.T) {
		_, err := client.GetMultipleOrderBooks([]types.BookParams{})
		if err == nil {
			t.Error("Expected error for empty requests array")
		}
	})
}

func TestGetUSDCBalance(t *testing.T) {
	client := newTestClobClient(t)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		balance, err := client.GetUSDCBalance()
		if err != nil {
			t.Fatalf("GetUSDCBalance failed: %v", err)
		}
		if balance < 0 {
			t.Errorf("Expected non-negative balance, got %f", balance)
		}
		t.Logf("GetUSDCBalance returned: %f", balance)
	})
}

func TestGetMidpoint(t *testing.T) {
	client := newTestClobClient(t)
	config := test.LoadTestConfig()

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		midpoint, err := client.GetMidpoint(config.TestTokenID)
		if err != nil {
			t.Fatalf("GetMidpoint failed: %v", err)
		}
		if midpoint == nil {
			t.Fatal("GetMidpoint returned nil")
		}
		t.Logf("GetMidpoint returned: %+v", midpoint)
	})
}

func TestGetMidpoints(t *testing.T) {
	client := newTestClobClient(t)
	config := test.LoadTestConfig()

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		tokenIDs := []string{config.TestTokenID}
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
}

func TestGetPrice(t *testing.T) {
	client := newTestClobClient(t)
	config := test.LoadTestConfig()

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// BUY价格测试
	t.Run("BUY", func(t *testing.T) {
		price, err := client.GetPrice(config.TestTokenID, types.OrderSideBUY)
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
		price, err := client.GetPrice(config.TestTokenID, types.OrderSideSELL)
		if err != nil {
			t.Fatalf("GetPrice SELL failed: %v", err)
		}
		if price == nil {
			t.Fatal("GetPrice returned nil")
		}
		t.Logf("GetPrice SELL returned: %+v", price)
	})
}

func TestGetPrices(t *testing.T) {
	client := newTestClobClient(t)
	config := test.LoadTestConfig()

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		requests := []types.BookParams{
			{TokenID: config.TestTokenID, Side: "BUY"},
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
}

func TestGetSpread(t *testing.T) {
	client := newTestClobClient(t)
	config := test.LoadTestConfig()

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		spread, err := client.GetSpread(config.TestTokenID)
		if err != nil {
			t.Fatalf("GetSpread failed: %v", err)
		}
		if spread == nil {
			t.Fatal("GetSpread returned nil")
		}
		t.Logf("GetSpread returned: %+v", spread)
	})
}

func TestGetSpreads(t *testing.T) {
	client := newTestClobClient(t)
	config := test.LoadTestConfig()

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		tokenIDs := []string{config.TestTokenID}
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
}

func TestGetLastTradePrice(t *testing.T) {
	client := newTestClobClient(t)
	config := test.LoadTestConfig()

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		lastTradePrice, err := client.GetLastTradePrice(config.TestTokenID)
		if err != nil {
			t.Fatalf("GetLastTradePrice failed: %v", err)
		}
		if lastTradePrice == nil {
			t.Fatal("GetLastTradePrice returned nil")
		}
		t.Logf("GetLastTradePrice returned: %+v", lastTradePrice)
	})
}

func TestGetLastTradesPrices(t *testing.T) {
	client := newTestClobClient(t)
	config := test.LoadTestConfig()

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		tokenIDs := []string{config.TestTokenID}
		lastTradesPrices, err := client.GetLastTradesPrices(tokenIDs)
		if err != nil {
			t.Fatalf("GetLastTradesPrices failed: %v", err)
		}
		if lastTradesPrices == nil {
			t.Fatal("GetLastTradesPrices returned nil")
		}
		if len(lastTradesPrices) != 1 {
			t.Errorf("Expected 1 last trade price, got %d", len(lastTradesPrices))
		}
		t.Logf("GetLastTradesPrices returned %d last trade prices", len(lastTradesPrices))
	})
}

func TestGetFeeRate(t *testing.T) {
	client := newTestClobClient(t)
	config := test.LoadTestConfig()

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		feeRate, err := client.GetFeeRate(config.TestTokenID)
		if err != nil {
			t.Fatalf("GetFeeRate failed: %v", err)
		}
		if feeRate < 0 {
			t.Errorf("Expected non-negative fee rate, got %d", feeRate)
		}
		t.Logf("GetFeeRate returned: %d", feeRate)
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
}
