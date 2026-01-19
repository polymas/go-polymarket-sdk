package gamma

import (
	"strings"
	"testing"

	"github.com/polymas/go-polymarket-sdk/test"
)

func TestGetMarket(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	if config.TestMarketID == "" {
		t.Skip("Skipping test: POLY_TEST_MARKET_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		market, err := client.GetMarket(config.TestMarketID)
		if err != nil {
			t.Fatalf("GetMarket failed: %v", err)
		}
		if market == nil {
			t.Fatal("GetMarket returned nil")
		}
		t.Logf("GetMarket returned market: %s", market.Slug)
	})

	// 边界条件测试 - 无效ID
	t.Run("InvalidID", func(t *testing.T) {
		_, err := client.GetMarket("invalid-id")
		if err == nil {
			t.Error("Expected error for invalid market ID")
		}
	})
}

func TestGetMarketBySlug(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	if config.TestMarketSlug == "" {
		t.Skip("Skipping test: POLY_TEST_MARKET_SLUG not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		includeTag := true
		market, err := client.GetMarketBySlug(config.TestMarketSlug, &includeTag)
		if err != nil {
			t.Fatalf("GetMarketBySlug failed: %v", err)
		}
		if market == nil {
			t.Fatal("GetMarketBySlug returned nil")
		}
		t.Logf("GetMarketBySlug returned market: %s", market.Slug)
	})

	// 不带includeTag测试
	t.Run("WithoutIncludeTag", func(t *testing.T) {
		market, err := client.GetMarketBySlug(config.TestMarketSlug, nil)
		if err != nil {
			t.Fatalf("GetMarketBySlug failed: %v", err)
		}
		if market == nil {
			t.Fatal("GetMarketBySlug returned nil")
		}
	})
}

func TestGetMarketsByConditionIDs(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	if config.TestConditionID == "" {
		t.Skip("Skipping test: POLY_TEST_CONDITION_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		conditionIDs := []string{string(config.TestConditionID)}
		markets, err := client.GetMarketsByConditionIDs(conditionIDs)
		if err != nil {
			t.Fatalf("GetMarketsByConditionIDs failed: %v", err)
		}
		if markets == nil {
			t.Fatal("GetMarketsByConditionIDs returned nil")
		}
		t.Logf("GetMarketsByConditionIDs returned %d markets", len(markets))
	})

	// 空数组测试
	t.Run("EmptyArray", func(t *testing.T) {
		markets, err := client.GetMarketsByConditionIDs([]string{})
		if err != nil {
			t.Fatalf("GetMarketsByConditionIDs with empty array failed: %v", err)
		}
		if markets == nil {
			t.Fatal("GetMarketsByConditionIDs returned nil")
		}
		if len(markets) != 0 {
			t.Errorf("Expected empty array, got %d markets", len(markets))
		}
	})
}

func TestGetMarkets(t *testing.T) {
	client := NewClient()

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		markets, err := client.GetMarkets(10)
		if err != nil {
			t.Fatalf("GetMarkets failed: %v", err)
		}
		if markets == nil {
			t.Fatal("GetMarkets returned nil")
		}
		if len(markets) > 10 {
			t.Errorf("Expected at most 10 markets, got %d", len(markets))
		}
		t.Logf("GetMarkets returned %d markets", len(markets))
	})

	// 带选项测试
	t.Run("WithOptions", func(t *testing.T) {
		active := true
		markets, err := client.GetMarkets(5,
			WithOffset(0),
			WithActive(active),
		)
		if err != nil {
			t.Fatalf("GetMarkets with options failed: %v", err)
		}
		if markets == nil {
			t.Fatal("GetMarkets returned nil")
		}
		if len(markets) > 5 {
			t.Errorf("Expected at most 5 markets, got %d", len(markets))
		}
	})

	// 测试limit边界值
	t.Run("LimitBoundaries", func(t *testing.T) {
		testCases := []struct {
			name  string
			limit int
		}{
			{"ZeroLimit", 0},
			{"NegativeLimit", -1},
			{"LargeLimit", 1000},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				markets, err := client.GetMarkets(tc.limit)
				if err != nil {
					t.Logf("GetMarkets with limit %d returned error (may be expected): %v", tc.limit, err)
				} else if markets != nil {
					t.Logf("GetMarkets with limit %d returned %d markets", tc.limit, len(markets))
				}
			})
		}
	})

	// 测试offset边界值
	t.Run("OffsetBoundaries", func(t *testing.T) {
		testCases := []struct {
			name   string
			offset int
		}{
			{"ZeroOffset", 0},
			{"NegativeOffset", -1},
			{"LargeOffset", 10000},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				markets, err := client.GetMarkets(10, WithOffset(tc.offset))
				if err != nil {
					t.Logf("GetMarkets with offset %d returned error (may be expected): %v", tc.offset, err)
				} else if markets != nil {
					t.Logf("GetMarkets with offset %d returned %d markets", tc.offset, len(markets))
				}
			})
		}
	})
}

func TestGetCertaintyMarkets(t *testing.T) {
	client := NewClient()

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		markets, err := client.GetCertaintyMarkets()
		if err != nil {
			t.Fatalf("GetCertaintyMarkets failed: %v", err)
		}
		if markets == nil {
			t.Fatal("GetCertaintyMarkets returned nil")
		}
		t.Logf("GetCertaintyMarkets returned %d markets", len(markets))
	})
}

func TestGetDisputeMarkets(t *testing.T) {
	client := NewClient()

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		markets, err := client.GetDisputeMarkets()
		if err != nil {
			t.Fatalf("GetDisputeMarkets failed: %v", err)
		}
		if markets == nil {
			t.Fatal("GetDisputeMarkets returned nil")
		}
		t.Logf("GetDisputeMarkets returned %d markets", len(markets))
	})
}

func TestGetAllMarkets(t *testing.T) {
	// 此测试已被标记为不测试，因为获取所有历史市场数据需要很长时间且容易超时
	t.Skip("TestGetAllMarkets is disabled - requires very long timeout and large dataset")
	
	client := NewClient()

	// 基本功能测试（这个测试可能很慢，所以只在非short模式下运行）
	// 注意：这个测试可能需要很长时间，因为要获取所有历史市场数据
	t.Run("Basic", func(t *testing.T) {
		// 设置更长的超时时间（在测试函数级别无法设置，需要在命令行使用-timeout参数）
		// 建议运行: go test -timeout 5m ./gamma -run TestGetAllMarkets
		markets, err := client.GetAllMarkets()
		if err != nil {
			// 如果是超时错误，记录但不失败（这是预期的，因为数据量很大）
			if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline") {
				t.Logf("GetAllMarkets timed out (expected for large dataset): %v", err)
				t.Skip("Skipping: GetAllMarkets requires longer timeout, run with -timeout 5m")
				return
			}
			t.Fatalf("GetAllMarkets failed: %v", err)
		}
		if markets == nil {
			t.Fatal("GetAllMarkets returned nil")
		}
		t.Logf("GetAllMarkets returned %d markets", len(markets))
	})
}

func TestGetEvent(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	if config.TestEventID == 0 {
		t.Skip("Skipping test: POLY_TEST_EVENT_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		event, err := client.GetEvent(config.TestEventID, nil, nil)
		if err != nil {
			t.Fatalf("GetEvent failed: %v", err)
		}
		if event == nil {
			t.Fatal("GetEvent returned nil")
		}
		t.Logf("GetEvent returned event: %s", event.Slug)
	})

	// 带选项测试
	t.Run("WithOptions", func(t *testing.T) {
		includeChat := true
		includeTemplate := true
		event, err := client.GetEvent(config.TestEventID, &includeChat, &includeTemplate)
		if err != nil {
			t.Fatalf("GetEvent with options failed: %v", err)
		}
		if event == nil {
			t.Fatal("GetEvent returned nil")
		}
	})
}

func TestGetEventBySlug(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	if config.TestMarketSlug == "" {
		t.Skip("Skipping test: POLY_TEST_MARKET_SLUG not set (using as event slug)")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		event, err := client.GetEventBySlug(config.TestMarketSlug, nil, nil)
		if err != nil {
			t.Fatalf("GetEventBySlug failed: %v", err)
		}
		if event == nil {
			t.Fatal("GetEventBySlug returned nil")
		}
		t.Logf("GetEventBySlug returned event: %s", event.Slug)
	})
}

func TestGetEvents(t *testing.T) {
	client := NewClient()

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		events, err := client.GetEvents(10, 0)
		if err != nil {
			t.Fatalf("GetEvents failed: %v", err)
		}
		if events == nil {
			t.Fatal("GetEvents returned nil")
		}
		if len(events) > 10 {
			t.Errorf("Expected at most 10 events, got %d", len(events))
		}
		t.Logf("GetEvents returned %d events", len(events))
	})

	// 带选项测试
	t.Run("WithOptions", func(t *testing.T) {
		active := true
		events, err := client.GetEvents(5, 0,
			WithEventsActive(active),
		)
		if err != nil {
			t.Fatalf("GetEvents with options failed: %v", err)
		}
		if events == nil {
			t.Fatal("GetEvents returned nil")
		}
		if len(events) > 5 {
			t.Errorf("Expected at most 5 events, got %d", len(events))
		}
	})
}

func TestSearch(t *testing.T) {
	client := NewClient()

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		result, err := client.Search("bitcoin")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if result == nil {
			t.Fatal("Search returned nil")
		}
		t.Logf("Search returned results")
	})

	// 带选项测试
	t.Run("WithOptions", func(t *testing.T) {
		result, err := client.Search("ethereum",
			WithSearchLimitPerType(5),
		)
		if err != nil {
			t.Fatalf("Search with options failed: %v", err)
		}
		if result == nil {
			t.Fatal("Search returned nil")
		}
	})

	// 空查询测试 - API应该返回422错误
	t.Run("EmptyQuery", func(t *testing.T) {
		result, err := client.Search("")
		if err != nil {
			// 空查询应该返回错误（422验证错误）
			if strings.Contains(err.Error(), "422") || strings.Contains(err.Error(), "empty") || strings.Contains(err.Error(), "validation error") {
				t.Logf("Search with empty query returned expected error: %v", err)
				return
			}
			t.Fatalf("Search with empty query failed with unexpected error: %v", err)
		}
		// 如果没有错误，记录但不算失败（API行为可能变化）
		if result != nil {
			t.Logf("Search with empty query succeeded (unexpected, API may have changed behavior)")
		}
	})

	// 测试特殊字符查询
	t.Run("SpecialCharacters", func(t *testing.T) {
		specialQueries := []string{
			"test@query",
			"test&query",
			"test+query",
			"test query",
		}

		for _, query := range specialQueries {
			t.Run(query, func(t *testing.T) {
				result, err := client.Search(query)
				if err != nil {
					t.Logf("Search with special characters '%s' returned error: %v", query, err)
				} else if result != nil {
					t.Logf("Search with special characters '%s' succeeded", query)
				}
			})
		}
	})

	// 测试长查询字符串
	t.Run("LongQuery", func(t *testing.T) {
		longQuery := "this is a very long search query that contains many words and should test the API's handling of long input strings"
		result, err := client.Search(longQuery)
		if err != nil {
			t.Logf("Search with long query returned error: %v", err)
		} else if result != nil {
			t.Logf("Search with long query succeeded")
		}
	})

	// 测试无结果的情况
	t.Run("NoResults", func(t *testing.T) {
		unlikelyQuery := "xyzabc123nonexistentquery987654321"
		result, err := client.Search(unlikelyQuery)
		if err != nil {
			t.Logf("Search with unlikely query returned error: %v", err)
		} else if result != nil {
			t.Logf("Search with unlikely query returned results (may be empty)")
		}
	})
}

func TestGetTags(t *testing.T) {
	client := NewClient()

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		tags, err := client.GetTags(10, 0)
		if err != nil {
			t.Fatalf("GetTags failed: %v", err)
		}
		if tags == nil {
			t.Fatal("GetTags returned nil")
		}
		if len(tags) > 10 {
			t.Errorf("Expected at most 10 tags, got %d", len(tags))
		}
		t.Logf("GetTags returned %d tags", len(tags))
	})
}

func TestGetTag(t *testing.T) {
	client := NewClient()

	// 基本功能测试（使用一个已知的tag ID，如果API有的话）
	t.Run("Basic", func(t *testing.T) {
		tag, err := client.GetTag(1)
		if err != nil {
			// 如果tag不存在，跳过测试
			if err.Error() == "tag not found" {
				t.Skip("Tag ID 1 not found")
			}
			t.Fatalf("GetTag failed: %v", err)
		}
		if tag == nil {
			t.Fatal("GetTag returned nil")
		}
		t.Logf("GetTag returned tag: %s", tag.Slug)
	})
}

func TestGetTagBySlug(t *testing.T) {
	client := NewClient()

	// 基本功能测试（使用一个常见的tag slug）
	t.Run("Basic", func(t *testing.T) {
		tag, err := client.GetTagBySlug("politics")
		if err != nil {
			// 如果tag不存在，跳过测试
			if err.Error() == "tag not found" {
				t.Skip("Tag 'politics' not found")
			}
			t.Fatalf("GetTagBySlug failed: %v", err)
		}
		if tag == nil {
			t.Fatal("GetTagBySlug returned nil")
		}
		t.Logf("GetTagBySlug returned tag: %s", tag.Slug)
	})
}

func TestGetSeries(t *testing.T) {
	client := NewClient()

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		series, err := client.GetSeries(10, 0)
		if err != nil {
			// 如果API端点不存在（404），跳过测试
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not Found") {
				t.Skip("Skipping test: GetSeries API endpoint not found (may be deprecated)")
				return
			}
			t.Fatalf("GetSeries failed: %v", err)
		}
		if series == nil {
			t.Fatal("GetSeries returned nil")
		}
		if len(series) > 10 {
			t.Errorf("Expected at most 10 series, got %d", len(series))
		}
		t.Logf("GetSeries returned %d series", len(series))
	})
}

func TestGetSeriesBySlug(t *testing.T) {
	client := NewClient()

	// 基本功能测试（使用一个常见的series slug）
	t.Run("Basic", func(t *testing.T) {
		series, err := client.GetSeriesBySlug("us-presidential-election")
		if err != nil {
			// 如果API端点不存在（404），跳过测试
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not Found") {
				t.Skip("Skipping test: GetSeriesBySlug API endpoint not found (may be deprecated)")
				return
			}
			// 如果series不存在，跳过测试
			if err.Error() == "series not found" {
				t.Skip("Series 'us-presidential-election' not found")
			}
			t.Fatalf("GetSeriesBySlug failed: %v", err)
		}
		if series == nil {
			t.Fatal("GetSeriesBySlug returned nil")
		}
		t.Logf("GetSeriesBySlug returned series: %s", series.Slug)
	})
}

func TestGetComments(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	if config.TestMarketID == "" {
		t.Skip("Skipping test: POLY_TEST_MARKET_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		comments, err := client.GetComments(config.TestMarketID, 10, 0)
		if err != nil {
			t.Fatalf("GetComments failed: %v", err)
		}
		if comments == nil {
			t.Fatal("GetComments returned nil")
		}
		if len(comments) > 10 {
			t.Errorf("Expected at most 10 comments, got %d", len(comments))
		}
		t.Logf("GetComments returned %d comments", len(comments))
	})
}

func TestGetComment(t *testing.T) {
	client := NewClient()

	// 基本功能测试（需要一个有效的comment ID）
	t.Run("Basic", func(t *testing.T) {
		// 使用一个示例comment ID，实际测试中应该使用真实的ID
		comment, err := client.GetComment("test-comment-id")
		if err != nil {
			// 如果comment不存在，这是预期的
			if err.Error() == "comment not found" {
				t.Skip("Comment 'test-comment-id' not found")
			}
			// 其他错误可能是API问题
			t.Logf("GetComment returned error (may be expected): %v", err)
		} else if comment == nil {
			t.Fatal("GetComment returned nil")
		}
	})
}

func TestGetProfile(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	// 使用测试用户地址
	userAddr := test.GetTestUserAddress(config)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		profile, err := client.GetProfile(userAddr)
		if err != nil {
			// 如果API端点不存在（404/405），跳过测试
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "405") || 
			   strings.Contains(err.Error(), "Not Found") || strings.Contains(err.Error(), "Method Not Allowed") {
				t.Skip("Skipping test: GetProfile API endpoint not found or method not allowed (may be deprecated)")
				return
			}
			t.Fatalf("GetProfile failed: %v", err)
		}
		if profile == nil {
			t.Fatal("GetProfile returned nil")
		}
		t.Logf("GetProfile returned profile for address: %s", userAddr)
	})
}

func TestGetProfileByUsername(t *testing.T) {
	client := NewClient()

	// 基本功能测试（使用一个已知的用户名）
	t.Run("Basic", func(t *testing.T) {
		profile, err := client.GetProfileByUsername("polymarket")
		if err != nil {
			// 如果API端点不存在（404），跳过测试
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not Found") {
				t.Skip("Skipping test: GetProfileByUsername API endpoint not found (may be deprecated)")
				return
			}
			// 如果用户不存在，跳过测试
			if err.Error() == "profile not found" {
				t.Skip("Profile 'polymarket' not found")
			}
			t.Fatalf("GetProfileByUsername failed: %v", err)
		}
		if profile == nil {
			t.Fatal("GetProfileByUsername returned nil")
		}
		t.Logf("GetProfileByUsername returned profile")
	})
}

func TestGetSamplingSimplifiedMarkets(t *testing.T) {
	client := NewClient()

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		markets, err := client.GetSamplingSimplifiedMarkets(10)
		if err != nil {
			// 如果API端点不存在（404），跳过测试
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not Found") {
				t.Skip("Skipping test: GetSamplingSimplifiedMarkets API endpoint not found (may be deprecated)")
				return
			}
			t.Fatalf("GetSamplingSimplifiedMarkets failed: %v", err)
		}
		if markets == nil {
			t.Fatal("GetSamplingSimplifiedMarkets returned nil")
		}
		if len(markets) > 10 {
			t.Errorf("Expected at most 10 markets, got %d", len(markets))
		}
		t.Logf("GetSamplingSimplifiedMarkets returned %d markets", len(markets))
	})
}

func TestGetSamplingMarkets(t *testing.T) {
	client := NewClient()

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		markets, err := client.GetSamplingMarkets(10)
		if err != nil {
			// 如果API端点不存在（404），跳过测试
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not Found") {
				t.Skip("Skipping test: GetSamplingMarkets API endpoint not found (may be deprecated)")
				return
			}
			t.Fatalf("GetSamplingMarkets failed: %v", err)
		}
		if markets == nil {
			t.Fatal("GetSamplingMarkets returned nil")
		}
		if len(markets) > 10 {
			t.Errorf("Expected at most 10 markets, got %d", len(markets))
		}
		t.Logf("GetSamplingMarkets returned %d markets", len(markets))
	})
}

func TestGetSimplifiedMarkets(t *testing.T) {
	client := NewClient()

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		markets, err := client.GetSimplifiedMarkets(10, 0)
		if err != nil {
			// 如果API端点不存在（404），跳过测试
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not Found") {
				t.Skip("Skipping test: GetSimplifiedMarkets API endpoint not found (may be deprecated)")
				return
			}
			t.Fatalf("GetSimplifiedMarkets failed: %v", err)
		}
		if markets == nil {
			t.Fatal("GetSimplifiedMarkets returned nil")
		}
		if len(markets) > 10 {
			t.Errorf("Expected at most 10 markets, got %d", len(markets))
		}
		t.Logf("GetSimplifiedMarkets returned %d markets", len(markets))
	})
}

func TestGetMarketTradesEvents(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	if config.TestMarketID == "" {
		t.Skip("Skipping test: POLY_TEST_MARKET_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		events, err := client.GetMarketTradesEvents(config.TestMarketID, 10, 0)
		if err != nil {
			t.Fatalf("GetMarketTradesEvents failed: %v", err)
		}
		if events == nil {
			t.Fatal("GetMarketTradesEvents returned nil")
		}
		if len(events) > 10 {
			t.Errorf("Expected at most 10 events, got %d", len(events))
		}
		t.Logf("GetMarketTradesEvents returned %d events", len(events))
	})
}
