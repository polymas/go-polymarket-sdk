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
}
