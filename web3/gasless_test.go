package web3

import (
	"testing"

	"github.com/polymas/go-polymarket-sdk/test"
	"github.com/polymas/go-polymarket-sdk/types"
)

// newTestGaslessClient 创建测试用的Gasless客户端
func newTestGaslessClient(t *testing.T) *GaslessClient {
	config := test.LoadTestConfig()
	test.SkipIfNoAuth(t, config)
	test.SkipIfNoBuilderCreds(t, config)

	builderCreds := &types.ApiCreds{
		Key:       config.BuilderAPIKey,
		Secret:    config.BuilderSecret,
		Passphrase: config.BuilderPassphrase,
	}

	client, err := NewGaslessClient(config.PrivateKey, config.SignatureType, config.ChainID, builderCreds)
	if err != nil {
		t.Fatalf("Failed to create Gasless client: %v", err)
	}

	return client
}

func TestRedeemPositions(t *testing.T) {
	client := newTestGaslessClient(t)
	config := test.LoadTestConfig()

	if config.TestConditionID == "" {
		t.Skip("Skipping test: POLY_TEST_CONDITION_ID not set")
	}

	// 注意：这个测试会实际赎回仓位，需要谨慎
	t.Run("Basic", func(t *testing.T) {
		positions := []RedeemPositionInfo{
			{
				ConditionID: config.TestConditionID,
				Amounts:     []float64{1.0, 0.0},
				NegRisk:     false,
			},
		}

		receipt, err := client.RedeemPositions(positions)
		if err != nil {
			t.Fatalf("RedeemPositions failed: %v", err)
		}
		if receipt == nil {
			t.Fatal("RedeemPositions returned nil")
		}
		t.Logf("RedeemPositions returned receipt: %+v", receipt)
	})

	// 边界条件测试 - 空数组
	t.Run("EmptyArray", func(t *testing.T) {
		_, err := client.RedeemPositions([]RedeemPositionInfo{})
		if err == nil {
			t.Error("Expected error for empty positions array")
		}
	})
}

func TestSplitUSDC(t *testing.T) {
	client := newTestGaslessClient(t)
	config := test.LoadTestConfig()

	if config.TestConditionID == "" {
		t.Skip("Skipping test: POLY_TEST_CONDITION_ID not set")
	}

	// 注意：这个测试会实际拆分USDC，需要谨慎
	t.Run("Basic", func(t *testing.T) {
		amount := 10.0
		receipt, err := client.SplitUSDC(amount, config.TestConditionID, false)
		if err != nil {
			t.Fatalf("SplitUSDC failed: %v", err)
		}
		if receipt == nil {
			t.Fatal("SplitUSDC returned nil")
		}
		t.Logf("SplitUSDC returned receipt: %+v", receipt)
	})

	// 边界条件测试 - 零金额
	t.Run("ZeroAmount", func(t *testing.T) {
		_, err := client.SplitUSDC(0.0, config.TestConditionID, false)
		if err == nil {
			t.Error("Expected error for zero amount")
		}
	})

	// 测试负数金额
	t.Run("NegativeAmount", func(t *testing.T) {
		_, err := client.SplitUSDC(-10.0, config.TestConditionID, false)
		if err == nil {
			t.Error("Expected error for negative amount")
		} else {
			t.Logf("SplitUSDC with negative amount returned error (expected): %v", err)
		}
	})

	// 测试极大金额
	t.Run("LargeAmount", func(t *testing.T) {
		largeAmount := 1e15
		_, err := client.SplitUSDC(largeAmount, config.TestConditionID, false)
		if err != nil {
			t.Logf("SplitUSDC with large amount returned error (may be expected): %v", err)
		} else {
			t.Logf("SplitUSDC with large amount succeeded")
		}
	})

	// 测试无效conditionID
	t.Run("InvalidConditionID", func(t *testing.T) {
		invalidConditionID := types.Keccak256("invalid-condition-id")
		_, err := client.SplitUSDC(10.0, invalidConditionID, false)
		if err != nil {
			t.Logf("SplitUSDC with invalid conditionID returned error (expected): %v", err)
		} else {
			t.Logf("SplitUSDC with invalid conditionID succeeded (may be acceptable)")
		}
	})
}

func TestMergeTokens(t *testing.T) {
	client := newTestGaslessClient(t)
	config := test.LoadTestConfig()

	if config.TestConditionID == "" {
		t.Skip("Skipping test: POLY_TEST_CONDITION_ID not set")
	}

	// 注意：这个测试会实际合并代币，需要谨慎
	t.Run("Basic", func(t *testing.T) {
		amount := 1.0
		receipt, err := client.MergeTokens(config.TestConditionID, amount, false)
		if err != nil {
			t.Fatalf("MergeTokens failed: %v", err)
		}
		if receipt == nil {
			t.Fatal("MergeTokens returned nil")
		}
		t.Logf("MergeTokens returned receipt: %+v", receipt)
	})

	// 边界条件测试 - 零或负数
	t.Run("ZeroOrNegative", func(t *testing.T) {
		_, err := client.MergeTokens(config.TestConditionID, 0.0, false)
		if err == nil {
			t.Error("Expected error for zero amount")
		}
		_, err = client.MergeTokens(config.TestConditionID, -1.0, false)
		if err == nil {
			t.Error("Expected error for negative amount")
		}
	})

	// 测试不同金额
	t.Run("DifferentAmounts", func(t *testing.T) {
		amounts := []float64{0.5, 1.0, 2.0}
		for _, amount := range amounts {
			receipt, err := client.MergeTokens(config.TestConditionID, amount, false)
			if err != nil {
				t.Logf("MergeTokens with amount %f failed (may be expected): %v", amount, err)
			} else if receipt != nil {
				t.Logf("MergeTokens with amount %f succeeded", amount)
			}
		}
	})

	// 测试无效conditionID
	t.Run("InvalidConditionID", func(t *testing.T) {
		invalidConditionID := types.Keccak256("invalid-condition-id")
		_, err := client.MergeTokens(invalidConditionID, 1.0, false)
		if err != nil {
			t.Logf("MergeTokens with invalid conditionID returned error (expected): %v", err)
		} else {
			t.Logf("MergeTokens with invalid conditionID succeeded (may be acceptable)")
		}
	})
}
