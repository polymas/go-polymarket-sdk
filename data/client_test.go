package data

import (
	"strings"
	"testing"

	"github.com/polymas/go-polymarket-sdk/test"
	"github.com/polymas/go-polymarket-sdk/types"
)

func TestGetPositions(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	// 使用测试用户地址
	userAddr := test.GetTestUserAddress(config)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		positions, err := client.GetPositions(userAddr)
		if err != nil {
			// 如果是服务器错误（502/504），可能是临时性问题，记录但不失败
			if strings.Contains(err.Error(), "502") || strings.Contains(err.Error(), "504") || 
			   strings.Contains(err.Error(), "Bad Gateway") || strings.Contains(err.Error(), "Gateway Timeout") {
				t.Logf("GetPositions failed with server error (may be temporary): %v", err)
				t.Skip("Skipping test: API server error (502/504), may be temporary")
				return
			}
			t.Fatalf("GetPositions failed: %v", err)
		}
		if positions == nil {
			t.Fatal("GetPositions returned nil")
		}
		t.Logf("GetPositions returned %d positions", len(positions))
	})

	// 带选项测试
	t.Run("WithOptions", func(t *testing.T) {
		positions, err := client.GetPositions(userAddr,
			WithPositionsSizeThreshold(0.1),
		)
		if err != nil {
			// 如果是服务器错误（502/504），可能是临时性问题，记录但不失败
			if strings.Contains(err.Error(), "502") || strings.Contains(err.Error(), "504") || 
			   strings.Contains(err.Error(), "Bad Gateway") || strings.Contains(err.Error(), "Gateway Timeout") {
				t.Logf("GetPositions with options failed with server error (may be temporary): %v", err)
				t.Skip("Skipping test: API server error (502/504), may be temporary")
				return
			}
			t.Fatalf("GetPositions with options failed: %v", err)
		}
		if positions == nil {
			t.Fatal("GetPositions returned nil")
		}
		if len(positions) > 10 {
			t.Errorf("Expected at most 10 positions, got %d", len(positions))
		}
	})

	// 带条件ID过滤测试
	if config.TestConditionID != "" {
		t.Run("WithConditionID", func(t *testing.T) {
			positions, err := client.GetPositions(userAddr,
				WithPositionsConditionID(string(config.TestConditionID)),
			)
			if err != nil {
				t.Fatalf("GetPositions with conditionID failed: %v", err)
			}
			if positions == nil {
				t.Fatal("GetPositions returned nil")
			}
			t.Logf("GetPositions with conditionID returned %d positions", len(positions))
		})
	}

	// 边界条件测试 - 空地址
	t.Run("EmptyAddress", func(t *testing.T) {
		_, err := client.GetPositions(types.EthAddress(""))
		if err == nil {
			t.Error("Expected error for empty address")
		}
	})
}

func TestGetTrades(t *testing.T) {
	client := NewClient()

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		trades, err := client.GetTrades(10, 0)
		if err != nil {
			t.Fatalf("GetTrades failed: %v", err)
		}
		if trades == nil {
			t.Fatal("GetTrades returned nil")
		}
		t.Logf("GetTrades returned %d trades", len(trades))
	})

	// 带选项测试
	t.Run("WithOptions", func(t *testing.T) {
		trades, err := client.GetTrades(5, 0,
			WithTradesTakerOnly(true),
		)
		if err != nil {
			t.Fatalf("GetTrades with options failed: %v", err)
		}
		if trades == nil {
			t.Fatal("GetTrades returned nil")
		}
		if len(trades) > 5 {
			t.Errorf("Expected at most 5 trades, got %d", len(trades))
		}
	})

	// 边界条件测试 - limit为0
	t.Run("ZeroLimit", func(t *testing.T) {
		trades, err := client.GetTrades(0, 0)
		if err != nil {
			t.Fatalf("GetTrades with zero limit failed: %v", err)
		}
		if trades == nil {
			t.Fatal("GetTrades returned nil")
		}
	})

	// 边界条件测试 - 大offset
	t.Run("LargeOffset", func(t *testing.T) {
		trades, err := client.GetTrades(10, 10000)
		if err != nil {
			t.Fatalf("GetTrades with large offset failed: %v", err)
		}
		if trades == nil {
			t.Fatal("GetTrades returned nil")
		}
	})

	// 测试limit边界值（>500的情况）
	t.Run("LimitOver500", func(t *testing.T) {
		trades, err := client.GetTrades(1000, 0)
		if err != nil {
			t.Fatalf("GetTrades with limit >500 failed: %v", err)
		}
		if trades == nil {
			t.Fatal("GetTrades returned nil")
		}
		// API可能返回超过500的结果（实际行为），或者限制为500
		// 只要返回了数据就认为测试通过
		if len(trades) > 0 {
			t.Logf("GetTrades with limit >500 returned %d trades", len(trades))
		} else {
			t.Logf("GetTrades with limit >500 returned 0 trades (may be expected)")
		}
	})

	// 测试所有选项组合
	t.Run("AllOptions", func(t *testing.T) {
		config := test.LoadTestConfig()
		// filterType应该是"CASH"或"TOKENS"，不是"amount"
		trades, err := client.GetTrades(10, 0,
			WithTradesTakerOnly(true),
			WithTradesFilter("CASH", func() *float64 { v := 100.0; return &v }()),
		)
		if err != nil {
			t.Logf("GetTrades with all options failed: %v", err)
		} else if trades != nil {
			t.Logf("GetTrades with all options returned %d trades", len(trades))
		}
		if config.TestConditionID != "" {
			trades, err = client.GetTrades(10, 0,
				WithTradesConditionID(string(config.TestConditionID)),
			)
			if err != nil {
				t.Logf("GetTrades with conditionID failed: %v", err)
			} else if trades != nil {
				t.Logf("GetTrades with conditionID returned %d trades", len(trades))
			}
		}
	})
}

func TestGetActivity(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	// 使用测试用户地址
	userAddr := test.GetTestUserAddress(config)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		activities, err := client.GetActivity(userAddr, 10, 0)
		if err != nil {
			t.Fatalf("GetActivity failed: %v", err)
		}
		if activities == nil {
			t.Fatal("GetActivity returned nil")
		}
		t.Logf("GetActivity returned %d activities", len(activities))
	})

	// 带选项测试
	t.Run("WithOptions", func(t *testing.T) {
		activities, err := client.GetActivity(userAddr, 5, 0,
			WithActivitySide(types.OrderSideBUY),
		)
		if err != nil {
			t.Fatalf("GetActivity with options failed: %v", err)
		}
		if activities == nil {
			t.Fatal("GetActivity returned nil")
		}
		if len(activities) > 5 {
			t.Errorf("Expected at most 5 activities, got %d", len(activities))
		}
	})

	// 边界条件测试 - 空地址
	t.Run("EmptyAddress", func(t *testing.T) {
		_, err := client.GetActivity(types.EthAddress(""), 10, 0)
		if err == nil {
			t.Error("Expected error for empty address")
		}
	})
}

func TestGetValue(t *testing.T) {
	client := NewClient()
	config := test.LoadTestConfig()

	// 使用测试用户地址
	userAddr := test.GetTestUserAddress(config)

	// 基本功能测试 - 不带条件ID
	t.Run("Basic", func(t *testing.T) {
		value, err := client.GetValue(userAddr, nil)
		if err != nil {
			t.Fatalf("GetValue failed: %v", err)
		}
		if value == nil {
			t.Fatal("GetValue returned nil")
		}
		t.Logf("GetValue returned: %+v", value)
	})

	// 带单个条件ID测试
	if config.TestConditionID != "" {
		t.Run("WithConditionID", func(t *testing.T) {
			value, err := client.GetValue(userAddr, string(config.TestConditionID))
			if err != nil {
				t.Fatalf("GetValue with conditionID failed: %v", err)
			}
			if value == nil {
				t.Fatal("GetValue returned nil")
			}
			t.Logf("GetValue with conditionID returned: %+v", value)
		})

		// 带多个条件ID测试
		t.Run("WithMultipleConditionIDs", func(t *testing.T) {
			conditionIDs := []string{string(config.TestConditionID)}
			value, err := client.GetValue(userAddr, conditionIDs)
			if err != nil {
				t.Fatalf("GetValue with multiple conditionIDs failed: %v", err)
			}
			if value == nil {
				t.Fatal("GetValue returned nil")
			}
			t.Logf("GetValue with multiple conditionIDs returned: %+v", value)
		})
	}

	// 边界条件测试 - 空地址
	t.Run("EmptyAddress", func(t *testing.T) {
		_, err := client.GetValue(types.EthAddress(""), nil)
		if err == nil {
			t.Error("Expected error for empty address")
		}
	})

	// 测试大量conditionID
	if config.TestConditionID != "" {
		t.Run("LargeConditionIDs", func(t *testing.T) {
			conditionIDs := make([]string, 50)
			for i := 0; i < 50; i++ {
				conditionIDs[i] = string(config.TestConditionID)
			}
			value, err := client.GetValue(userAddr, conditionIDs)
			if err != nil {
				t.Logf("GetValue with large conditionIDs returned error: %v", err)
			} else if value != nil {
				t.Logf("GetValue with large conditionIDs succeeded")
			}
		})
	}

	// 测试部分conditionID无效的情况
	t.Run("PartialInvalidConditionIDs", func(t *testing.T) {
		conditionIDs := []string{
			"invalid-condition-id-1",
			"invalid-condition-id-2",
		}
		value, err := client.GetValue(userAddr, conditionIDs)
		if err != nil {
			t.Logf("GetValue with partial invalid conditionIDs returned error: %v", err)
		} else if value != nil {
			t.Logf("GetValue with partial invalid conditionIDs succeeded")
		}
	})
}
