package gamma

import (
	"regexp"
	"strings"
	"testing"

	"github.com/polymas/go-polymarket-sdk/test"
)

// eventURLPattern 匹配 https://polymarket.com/event/{slug}(/{slug})?
var eventURLPattern = regexp.MustCompile(`^https://polymarket\.com/event/[a-z0-9][a-z0-9\-]*(?:/[a-z0-9][a-z0-9\-]*)?$`)

func assertEventURL(t *testing.T, url string) {
	t.Helper()
	if !eventURLPattern.MatchString(url) {
		t.Errorf("unexpected URL format: %q", url)
	}
}

func TestGetEventURLByMarketID(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	t.Run("EmptyMarketID", func(t *testing.T) {
		_, err := client.GetEventURLByMarketID("")
		if err == nil {
			t.Fatal("expected error for empty marketID")
		}
	})

	t.Run("InvalidMarketID", func(t *testing.T) {
		_, err := client.GetEventURLByMarketID("definitely-not-a-real-market-id-xxxxx")
		if err == nil {
			t.Fatal("expected error for invalid marketID")
		}
	})

	t.Run("Basic", func(t *testing.T) {
		if config.TestMarketID == "" {
			t.Skip("Skipping: POLY_TEST_MARKET_ID not set")
		}
		url, err := client.GetEventURLByMarketID(config.TestMarketID)
		if err != nil {
			t.Fatalf("GetEventURLByMarketID failed: %v", err)
		}
		assertEventURL(t, url)
		t.Logf("marketID=%s -> %s", config.TestMarketID, url)
	})
}

func TestGetEventURLByConditionID(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	t.Run("EmptyConditionID", func(t *testing.T) {
		_, err := client.GetEventURLByConditionID("")
		if err == nil {
			t.Fatal("expected error for empty conditionID")
		}
	})

	t.Run("UnknownConditionID", func(t *testing.T) {
		// 合法格式但极不可能存在的 condition id
		_, err := client.GetEventURLByConditionID("0x0000000000000000000000000000000000000000000000000000000000000001")
		if err == nil {
			t.Fatal("expected error for unknown conditionID")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Logf("got error (acceptable): %v", err)
		}
	})

	t.Run("Basic", func(t *testing.T) {
		if config.TestConditionID == "" {
			t.Skip("Skipping: POLY_TEST_CONDITION_ID not set")
		}
		url, err := client.GetEventURLByConditionID(string(config.TestConditionID))
		if err != nil {
			t.Fatalf("GetEventURLByConditionID failed: %v", err)
		}
		assertEventURL(t, url)
		t.Logf("conditionID=%s -> %s", config.TestConditionID, url)
	})
}

func TestGetEventURLByTokenID(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	t.Run("EmptyTokenID", func(t *testing.T) {
		_, err := client.GetEventURLByTokenID("")
		if err == nil {
			t.Fatal("expected error for empty tokenID")
		}
	})

	t.Run("Basic", func(t *testing.T) {
		if config.TestTokenID == "" {
			t.Skip("Skipping: POLY_TEST_TOKEN_ID not set")
		}
		url, err := client.GetEventURLByTokenID(config.TestTokenID)
		if err != nil {
			t.Fatalf("GetEventURLByTokenID failed: %v", err)
		}
		assertEventURL(t, url)
		t.Logf("tokenID=%s -> %s", config.TestTokenID, url)
	})
}

// TestGetEventURL_Consistency 验证对同一市场，三种查询方式（marketID / conditionID / tokenID）
// 返回的 URL 完全一致
func TestGetEventURL_Consistency(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	if config.TestMarketSlug == "" {
		t.Skip("Skipping: POLY_TEST_MARKET_SLUG not set")
	}

	market, err := client.GetMarketBySlug(config.TestMarketSlug)
	if err != nil {
		t.Fatalf("GetMarketBySlug failed: %v", err)
	}
	if market == nil {
		t.Fatal("GetMarketBySlug returned nil")
	}

	urlByMarketID, err := client.GetEventURLByMarketID(market.MarketID)
	if err != nil {
		t.Fatalf("GetEventURLByMarketID failed: %v", err)
	}
	assertEventURL(t, urlByMarketID)

	urlByConditionID, err := client.GetEventURLByConditionID(string(market.ConditionID))
	if err != nil {
		t.Fatalf("GetEventURLByConditionID failed: %v", err)
	}
	if urlByConditionID != urlByMarketID {
		t.Errorf("URL by conditionID mismatch:\n  byMarketID   =%s\n  byConditionID=%s", urlByMarketID, urlByConditionID)
	}

	if len(market.TokenIDs) > 0 {
		urlByTokenID, err := client.GetEventURLByTokenID(market.TokenIDs[0])
		if err != nil {
			t.Fatalf("GetEventURLByTokenID failed: %v", err)
		}
		if urlByTokenID != urlByMarketID {
			t.Errorf("URL by tokenID mismatch:\n  byMarketID=%s\n  byTokenID =%s", urlByMarketID, urlByTokenID)
		}
	}

	// 验证 URL 中包含 market.Slug（除非它与 event.Slug 相同，此时只有一段 slug）
	if !strings.Contains(urlByMarketID, market.Slug) {
		t.Errorf("URL %q does not contain market slug %q", urlByMarketID, market.Slug)
	}

	t.Logf("consistent URL: %s", urlByMarketID)
}
