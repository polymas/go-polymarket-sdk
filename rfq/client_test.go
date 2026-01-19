package rfq

import (
	"testing"

	"github.com/polymas/go-polymarket-sdk/test"
	"github.com/polymas/go-polymarket-sdk/types"
)

func TestRequestQuote(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	test.SkipIfNoAuth(t, config)

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 注意：这个测试会实际请求报价，需要谨慎
	t.Run("Basic", func(t *testing.T) {
		request := types.RFQRequest{
			TokenID: config.TestTokenID,
			Side:    types.OrderSideBUY,
			Size:    10.0,
		}

		response, err := client.RequestQuote(request)
		if err != nil {
			t.Fatalf("RequestQuote failed: %v", err)
		}
		if response == nil {
			t.Fatal("RequestQuote returned nil")
		}
		t.Logf("RequestQuote returned: %+v", response)
	})

	// 测试无效tokenID
	t.Run("InvalidTokenID", func(t *testing.T) {
		request := types.RFQRequest{
			TokenID: "invalid-token-id",
			Side:    types.OrderSideBUY,
			Size:    10.0,
		}

		_, err := client.RequestQuote(request)
		if err != nil {
			t.Logf("RequestQuote with invalid tokenID returned error (expected): %v", err)
		} else {
			t.Error("Expected error for invalid tokenID")
		}
	})

	// 测试负数size
	t.Run("NegativeSize", func(t *testing.T) {
		request := types.RFQRequest{
			TokenID: config.TestTokenID,
			Side:    types.OrderSideBUY,
			Size:    -10.0,
		}

		_, err := client.RequestQuote(request)
		if err != nil {
			t.Logf("RequestQuote with negative size returned error (expected): %v", err)
		} else {
			t.Error("Expected error for negative size")
		}
	})

	// 测试0 size
	t.Run("ZeroSize", func(t *testing.T) {
		request := types.RFQRequest{
			TokenID: config.TestTokenID,
			Side:    types.OrderSideBUY,
			Size:    0.0,
		}

		_, err := client.RequestQuote(request)
		if err != nil {
			t.Logf("RequestQuote with zero size returned error (expected): %v", err)
		} else {
			t.Error("Expected error for zero size")
		}
	})

	// 测试极大size
	t.Run("LargeSize", func(t *testing.T) {
		request := types.RFQRequest{
			TokenID: config.TestTokenID,
			Side:    types.OrderSideBUY,
			Size:    1e15,
		}

		response, err := client.RequestQuote(request)
		if err != nil {
			t.Logf("RequestQuote with large size returned error (may be expected): %v", err)
		} else if response != nil {
			t.Logf("RequestQuote with large size succeeded")
		}
	})
}

func TestGetQuotes(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	test.SkipIfNoAuth(t, config)

	// 注意：这个测试需要有效的请求ID
	t.Run("Basic", func(t *testing.T) {
		// 先创建一个请求
		if config.TestTokenID == "" {
			t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
		}

		request := types.RFQRequest{
			TokenID: config.TestTokenID,
			Side:    types.OrderSideBUY,
			Size:    10.0,
		}

		requestResponse, err := client.RequestQuote(request)
		if err != nil {
			t.Fatalf("RequestQuote failed: %v", err)
		}
		if requestResponse == nil || requestResponse.RequestID == "" {
			t.Skip("Skipping test: No request ID returned")
		}

		// 获取报价
		quotes, err := client.GetQuotes(requestResponse.RequestID)
		if err != nil {
			t.Fatalf("GetQuotes failed: %v", err)
		}
		if quotes == nil {
			t.Fatal("GetQuotes returned nil")
		}
		t.Logf("GetQuotes returned %d quotes", len(quotes))
	})

	// 测试无效requestID
	t.Run("InvalidRequestID", func(t *testing.T) {
		_, err := client.GetQuotes("invalid-request-id")
		if err != nil {
			t.Logf("GetQuotes with invalid requestID returned error (expected): %v", err)
		} else {
			t.Error("Expected error for invalid requestID")
		}
	})

	// 测试不存在的requestID
	t.Run("NonExistentRequestID", func(t *testing.T) {
		nonExistentID := "00000000-0000-0000-0000-000000000000"
		quotes, err := client.GetQuotes(nonExistentID)
		if err != nil {
			t.Logf("GetQuotes with non-existent requestID returned error: %v", err)
		} else if quotes != nil {
			if len(quotes) == 0 {
				t.Logf("GetQuotes with non-existent requestID returned empty array (expected)")
			} else {
				t.Logf("GetQuotes with non-existent requestID returned %d quotes", len(quotes))
			}
		}
	})
}

func TestAcceptQuote(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	test.SkipIfNoAuth(t, config)

	// 注意：这个测试需要有效的报价ID
	t.Run("Basic", func(t *testing.T) {
		// 先创建一个请求并获取报价
		if config.TestTokenID == "" {
			t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
		}

		request := types.RFQRequest{
			TokenID: config.TestTokenID,
			Side:    types.OrderSideBUY,
			Size:    10.0,
		}

		requestResponse, err := client.RequestQuote(request)
		if err != nil {
			t.Fatalf("RequestQuote failed: %v", err)
		}
		if requestResponse == nil || requestResponse.RequestID == "" {
			t.Skip("Skipping test: No request ID returned")
		}

		// 获取报价
		quotes, err := client.GetQuotes(requestResponse.RequestID)
		if err != nil {
			t.Fatalf("GetQuotes failed: %v", err)
		}
		if len(quotes) == 0 {
			t.Skip("Skipping test: No quotes available")
		}

		// 接受第一个报价
		response, err := client.AcceptQuote(quotes[0].QuoteID)
		if err != nil {
			t.Fatalf("AcceptQuote failed: %v", err)
		}
		if response == nil {
			t.Fatal("AcceptQuote returned nil")
		}
		t.Logf("AcceptQuote returned: %+v", response)
	})
}

func TestCancelRequest(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	test.SkipIfNoAuth(t, config)

	// 注意：这个测试需要有效的请求ID
	t.Run("Basic", func(t *testing.T) {
		// 先创建一个请求
		if config.TestTokenID == "" {
			t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
		}

		request := types.RFQRequest{
			TokenID: config.TestTokenID,
			Side:    types.OrderSideBUY,
			Size:    10.0,
		}

		requestResponse, err := client.RequestQuote(request)
		if err != nil {
			t.Fatalf("RequestQuote failed: %v", err)
		}
		if requestResponse == nil || requestResponse.RequestID == "" {
			t.Skip("Skipping test: No request ID returned")
		}

		// 取消请求
		err = client.CancelRequest(requestResponse.RequestID)
		if err != nil {
			t.Fatalf("CancelRequest failed: %v", err)
		}
		t.Logf("CancelRequest succeeded")
	})

	// 测试无效requestID
	t.Run("InvalidRequestID", func(t *testing.T) {
		err := client.CancelRequest("invalid-request-id")
		if err != nil {
			t.Logf("CancelRequest with invalid requestID returned error (expected): %v", err)
		} else {
			t.Logf("CancelRequest with invalid requestID succeeded (may be acceptable)")
		}
	})

	// 测试已取消的请求
	t.Run("AlreadyCanceled", func(t *testing.T) {
		if config.TestTokenID == "" {
			t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
		}

		request := types.RFQRequest{
			TokenID: config.TestTokenID,
			Side:    types.OrderSideBUY,
			Size:    10.0,
		}

		requestResponse, err := client.RequestQuote(request)
		if err != nil {
			t.Fatalf("RequestQuote failed: %v", err)
		}
		if requestResponse == nil || requestResponse.RequestID == "" {
			t.Skip("Skipping test: No request ID returned")
		}

		// 第一次取消
		err = client.CancelRequest(requestResponse.RequestID)
		if err != nil {
			t.Logf("First cancel failed: %v", err)
			return
		}

		// 再次尝试取消同一个请求
		err = client.CancelRequest(requestResponse.RequestID)
		if err != nil {
			t.Logf("CancelRequest with already canceled request returned error (expected): %v", err)
		} else {
			t.Logf("CancelRequest with already canceled request succeeded (may be acceptable)")
		}
	})
}
