package data

import (
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
}
