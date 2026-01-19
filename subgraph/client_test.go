package subgraph

import (
	"testing"
	"time"

	"github.com/polymas/go-polymarket-sdk/test"
)

func TestQuery(t *testing.T) {
	client := NewClient()

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		query := `
			query {
				markets(first: 5) {
					id
					slug
				}
			}
		`
		variables := map[string]interface{}{}

		response, err := client.Query(query, variables)
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if response == nil {
			t.Fatal("Query returned nil")
		}
		t.Logf("Query returned response")
	})
}

func TestGetMarketVolume(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	if config.TestMarketID == "" {
		t.Skip("Skipping test: POLY_TEST_MARKET_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		now := time.Now()
		startTime := now.Add(-7 * 24 * time.Hour).Unix() // 7天前
		endTime := now.Unix()

		volume, err := client.GetMarketVolume(config.TestMarketID, startTime, endTime)
		if err != nil {
			t.Fatalf("GetMarketVolume failed: %v", err)
		}
		if volume == nil {
			t.Fatal("GetMarketVolume returned nil")
		}
		t.Logf("GetMarketVolume returned: %+v", volume)
	})
}

func TestGetUserPositions(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	// 使用测试用户地址
	userAddr := test.GetTestUserAddress(config)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		positions, err := client.GetUserPositions(userAddr)
		if err != nil {
			t.Fatalf("GetUserPositions failed: %v", err)
		}
		if positions == nil {
			t.Fatal("GetUserPositions returned nil")
		}
		t.Logf("GetUserPositions returned %d positions", len(positions))
	})
}

func TestGetMarketOpenInterest(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	if config.TestMarketID == "" {
		t.Skip("Skipping test: POLY_TEST_MARKET_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		openInterest, err := client.GetMarketOpenInterest(config.TestMarketID)
		if err != nil {
			t.Fatalf("GetMarketOpenInterest failed: %v", err)
		}
		if openInterest == nil {
			t.Fatal("GetMarketOpenInterest returned nil")
		}
		t.Logf("GetMarketOpenInterest returned: %+v", openInterest)
	})
}

func TestGetUserPNL(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	// 使用测试用户地址
	userAddr := test.GetTestUserAddress(config)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		pnl, err := client.GetUserPNL(userAddr)
		if err != nil {
			t.Fatalf("GetUserPNL failed: %v", err)
		}
		if pnl == nil {
			t.Fatal("GetUserPNL returned nil")
		}
		t.Logf("GetUserPNL returned: %+v", pnl)
	})
}
