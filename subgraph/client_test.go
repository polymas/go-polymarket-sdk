package subgraph

import (
	"strings"
	"testing"
	"time"

	"github.com/polymas/go-polymarket-sdk/test"
	"github.com/polymas/go-polymarket-sdk/types"
)

func TestQuery(t *testing.T) {
	client := NewClient()

	// 基本功能测试
	// 注意：Subgraph API端点已被移除，测试会失败
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
			// 检查是否是端点已移除的错误
			if strings.Contains(err.Error(), "endpoint has been removed") || 
			   strings.Contains(err.Error(), "removed") {
				t.Skip("Skipping test: Subgraph endpoint has been removed by The Graph")
				return
			}
			t.Fatalf("Query failed: %v", err)
		}
		if response == nil {
			t.Fatal("Query returned nil")
		}
		t.Logf("Query returned response")
	})

	// 测试无效GraphQL查询
	t.Run("InvalidQuery", func(t *testing.T) {
		invalidQuery := "invalid graphql query syntax"
		variables := map[string]interface{}{}

		_, err := client.Query(invalidQuery, variables)
		if err != nil {
			// 检查是否是端点已移除的错误
			if strings.Contains(err.Error(), "endpoint has been removed") || 
			   strings.Contains(err.Error(), "removed") {
				t.Skip("Skipping test: Subgraph endpoint has been removed by The Graph")
				return
			}
			t.Logf("Query with invalid syntax returned error (expected): %v", err)
		} else {
			t.Error("Expected error for invalid query")
		}
	})

	// 测试无效变量
	t.Run("InvalidVariables", func(t *testing.T) {
		query := `
			query GetMarket($marketId: String!) {
				market(id: $marketId) {
					id
				}
			}
		`
		// 缺少必需的变量
		variables := map[string]interface{}{}

		_, err := client.Query(query, variables)
		if err != nil {
			t.Logf("Query with invalid variables returned error (expected): %v", err)
		} else {
			t.Logf("Query with invalid variables succeeded (may return GraphQL errors)")
		}
	})

	// 测试大查询
	t.Run("LargeQuery", func(t *testing.T) {
		// 创建一个包含很多字段的查询
		query := `
			query {
				markets(first: 100) {
					id
					slug
					title
					description
					resolutionSource
					endDate
					image
					icon
					active
					closed
					archived
					createdTimestamp
					lastActiveTimestamp
					volume
					liquidity
					outcomes
					tokens {
						tokenID
						outcome
						price
					}
				}
			}
		`
		variables := map[string]interface{}{}

		response, err := client.Query(query, variables)
		if err != nil {
			t.Logf("Large query returned error: %v", err)
		} else if response != nil {
			t.Logf("Large query succeeded")
		}
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

	// 测试无效marketID
	t.Run("InvalidMarketID", func(t *testing.T) {
		now := time.Now()
		startTime := now.Add(-7 * 24 * time.Hour).Unix()
		endTime := now.Unix()

		_, err := client.GetMarketVolume("invalid-market-id", startTime, endTime)
		if err != nil {
			t.Logf("GetMarketVolume with invalid marketID returned error (expected): %v", err)
		} else {
			t.Logf("GetMarketVolume with invalid marketID succeeded (may return nil)")
		}
	})

	// 测试无效时间范围（startTime > endTime）
	t.Run("InvalidTimeRange", func(t *testing.T) {
		now := time.Now()
		startTime := now.Unix()
		endTime := now.Add(-7 * 24 * time.Hour).Unix() // endTime在startTime之前

		_, err := client.GetMarketVolume(config.TestMarketID, startTime, endTime)
		if err != nil {
			t.Logf("GetMarketVolume with invalid time range returned error (expected): %v", err)
		} else {
			t.Logf("GetMarketVolume with invalid time range succeeded (may return empty result)")
		}
	})

	// 测试未来时间范围
	t.Run("FutureTimeRange", func(t *testing.T) {
		now := time.Now()
		startTime := now.Add(1 * 24 * time.Hour).Unix() // 未来
		endTime := now.Add(7 * 24 * time.Hour).Unix()

		volume, err := client.GetMarketVolume(config.TestMarketID, startTime, endTime)
		if err != nil {
			t.Logf("GetMarketVolume with future time range returned error: %v", err)
		} else if volume != nil {
			t.Logf("GetMarketVolume with future time range returned: %+v (may be empty)", volume)
		}
	})
}

func TestGetUserPositions(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	// 使用测试用户地址
	userAddr := test.GetTestUserAddress(config)
	
	// 注意：Subgraph API端点已被移除，测试会失败

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		positions, err := client.GetUserPositions(userAddr)
		if err != nil {
			// 检查是否是端点已移除的错误
			if strings.Contains(err.Error(), "endpoint has been removed") || 
			   strings.Contains(err.Error(), "removed") {
				t.Skip("Skipping test: Subgraph endpoint has been removed by The Graph")
				return
			}
			t.Fatalf("GetUserPositions failed: %v", err)
		}
		if positions == nil {
			t.Fatal("GetUserPositions returned nil")
		}
		t.Logf("GetUserPositions returned %d positions", len(positions))
	})

	// 测试无效地址格式
	t.Run("InvalidAddress", func(t *testing.T) {
		invalidAddr := types.EthAddress("invalid-address")
		_, err := client.GetUserPositions(invalidAddr)
		if err != nil {
			t.Logf("GetUserPositions with invalid address returned error (expected): %v", err)
		} else {
			t.Logf("GetUserPositions with invalid address succeeded (may return empty result)")
		}
	})

	// 测试无仓位的地址
	t.Run("NoPositions", func(t *testing.T) {
		// 使用一个不太可能有仓位的地址
		emptyAddr := types.EthAddress("0x0000000000000000000000000000000000000000")
		positions, err := client.GetUserPositions(emptyAddr)
		if err != nil {
			t.Logf("GetUserPositions with empty address returned error: %v", err)
		} else if positions != nil {
			if len(positions) == 0 {
				t.Logf("GetUserPositions with empty address returned empty array (expected)")
			} else {
				t.Logf("GetUserPositions with empty address returned %d positions", len(positions))
			}
		}
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
	// 注意：Subgraph API端点已被移除，测试会失败
	t.Run("Basic", func(t *testing.T) {
		pnl, err := client.GetUserPNL(userAddr)
		if err != nil {
			// 检查是否是端点已移除的错误
			if strings.Contains(err.Error(), "endpoint has been removed") || 
			   strings.Contains(err.Error(), "removed") {
				t.Skip("Skipping test: Subgraph endpoint has been removed by The Graph")
				return
			}
			t.Fatalf("GetUserPNL failed: %v", err)
		}
		if pnl == nil {
			t.Fatal("GetUserPNL returned nil")
		}
		t.Logf("GetUserPNL returned: %+v", pnl)
	})
}
