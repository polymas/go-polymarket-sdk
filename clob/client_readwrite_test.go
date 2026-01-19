package clob

import (
	"testing"

	"github.com/polymas/go-polymarket-sdk/test"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

// newTestClobClientWithAuth 创建需要认证的CLOB客户端
func newTestClobClientWithAuth(t *testing.T) Client {
	config := test.LoadTestConfig()
	test.SkipIfNoAuth(t, config)

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

func TestGetOrders(t *testing.T) {
	client := newTestClobClientWithAuth(t)

	// 基本功能测试 - 获取所有订单
	t.Run("Basic", func(t *testing.T) {
		orders, err := client.GetOrders(nil, nil, nil)
		if err != nil {
			t.Fatalf("GetOrders failed: %v", err)
		}
		if orders == nil {
			t.Fatal("GetOrders returned nil")
		}
		t.Logf("GetOrders returned %d orders", len(orders))
	})

	// 带条件ID过滤测试
	config := test.LoadTestConfig()
	if config.TestConditionID != "" {
		t.Run("WithConditionID", func(t *testing.T) {
			conditionID := config.TestConditionID
			orders, err := client.GetOrders(nil, &conditionID, nil)
			if err != nil {
				t.Fatalf("GetOrders with conditionID failed: %v", err)
			}
			if orders == nil {
				t.Fatal("GetOrders returned nil")
			}
			t.Logf("GetOrders with conditionID returned %d orders", len(orders))
		})
	}
}

func TestCreateAndPostOrders(t *testing.T) {
	client := newTestClobClientWithAuth(t)
	config := test.LoadTestConfig()

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 注意：这个测试会实际创建订单，需要谨慎
	t.Run("Basic", func(t *testing.T) {
		orderArgs := []types.OrderArgs{
			{
				TokenID: config.TestTokenID,
				Side:    types.OrderSideBUY,
				Price:   0.5,
				Size:    10.0,
			},
		}
		orderTypes := []types.OrderType{types.OrderTypeGTC}

		responses, err := client.CreateAndPostOrders(orderArgs, orderTypes)
		if err != nil {
			t.Fatalf("CreateAndPostOrders failed: %v", err)
		}
		if responses == nil {
			t.Fatal("CreateAndPostOrders returned nil")
		}
		if len(responses) != 1 {
			t.Errorf("Expected 1 response, got %d", len(responses))
		}

		// 检查响应
		if responses[0].ErrorMsg != "" {
			t.Logf("Order creation returned error (may be expected): %s", responses[0].ErrorMsg)
		} else {
			t.Logf("Order created successfully: %+v", responses[0])
		}
	})
}

func TestPostOrder(t *testing.T) {
	client := newTestClobClientWithAuth(t)
	config := test.LoadTestConfig()

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 注意：这个测试会实际创建订单
	t.Run("Basic", func(t *testing.T) {
		orderArgs := types.OrderArgs{
			TokenID: config.TestTokenID,
			Side:    types.OrderSideBUY,
			Price:   0.5,
			Size:    10.0,
		}

		response, err := client.PostOrder(orderArgs, types.OrderTypeGTC)
		if err != nil {
			t.Fatalf("PostOrder failed: %v", err)
		}
		if response == nil {
			t.Fatal("PostOrder returned nil")
		}

		if response.ErrorMsg != "" {
			t.Logf("Order creation returned error (may be expected): %s", response.ErrorMsg)
		} else {
			t.Logf("Order created successfully: %+v", response)
		}
	})
}

func TestCancelOrders(t *testing.T) {
	client := newTestClobClientWithAuth(t)

	// 注意：这个测试需要有效的订单ID
	t.Run("Basic", func(t *testing.T) {
		// 先获取一些订单
		orders, err := client.GetOrders(nil, nil, nil)
		if err != nil {
			t.Fatalf("GetOrders failed: %v", err)
		}

		if len(orders) == 0 {
			t.Skip("Skipping test: No orders to cancel")
		}

		// 取消第一个订单
		orderIDs := []types.Keccak256{orders[0].OrderID}
		response, err := client.CancelOrders(orderIDs)
		if err != nil {
			t.Fatalf("CancelOrders failed: %v", err)
		}
		if response == nil {
			t.Fatal("CancelOrders returned nil")
		}
		t.Logf("CancelOrders response: %+v", response)
	})

	// 空数组测试
	t.Run("EmptyArray", func(t *testing.T) {
		response, err := client.CancelOrders([]types.Keccak256{})
		if err != nil {
			t.Fatalf("CancelOrders with empty array failed: %v", err)
		}
		if response == nil {
			t.Fatal("CancelOrders returned nil")
		}
		if len(response.Canceled) != 0 {
			t.Errorf("Expected 0 canceled orders, got %d", len(response.Canceled))
		}
	})
}

func TestCancelOrder(t *testing.T) {
	client := newTestClobClientWithAuth(t)

	// 注意：这个测试需要有效的订单ID
	t.Run("Basic", func(t *testing.T) {
		// 先获取一些订单
		orders, err := client.GetOrders(nil, nil, nil)
		if err != nil {
			t.Fatalf("GetOrders failed: %v", err)
		}

		if len(orders) == 0 {
			t.Skip("Skipping test: No orders to cancel")
		}

		// 取消第一个订单
		response, err := client.CancelOrder(orders[0].OrderID)
		if err != nil {
			t.Fatalf("CancelOrder failed: %v", err)
		}
		if response == nil {
			t.Fatal("CancelOrder returned nil")
		}
		t.Logf("CancelOrder response: %+v", response)
	})
}

func TestCancelAll(t *testing.T) {
	client := newTestClobClientWithAuth(t)

	// 注意：这个测试会取消所有订单
	t.Run("Basic", func(t *testing.T) {
		response, err := client.CancelAll()
		if err != nil {
			t.Fatalf("CancelAll failed: %v", err)
		}
		if response == nil {
			t.Fatal("CancelAll returned nil")
		}
		t.Logf("CancelAll response: %+v", response)
	})
}

func TestCancelMarketOrders(t *testing.T) {
	client := newTestClobClientWithAuth(t)
	config := test.LoadTestConfig()

	if config.TestConditionID == "" {
		t.Skip("Skipping test: POLY_TEST_CONDITION_ID not set")
	}

	// 注意：这个测试会取消指定市场的所有订单
	t.Run("Basic", func(t *testing.T) {
		response, err := client.CancelMarketOrders(config.TestConditionID)
		if err != nil {
			t.Fatalf("CancelMarketOrders failed: %v", err)
		}
		if response == nil {
			t.Fatal("CancelMarketOrders returned nil")
		}
		t.Logf("CancelMarketOrders response: %+v", response)
	})
}

func TestGetBalanceAllowance(t *testing.T) {
	client := newTestClobClientWithAuth(t)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		balanceAllowance, err := client.GetBalanceAllowance()
		if err != nil {
			t.Fatalf("GetBalanceAllowance failed: %v", err)
		}
		if balanceAllowance == nil {
			t.Fatal("GetBalanceAllowance returned nil")
		}
		t.Logf("GetBalanceAllowance returned: %+v", balanceAllowance)
	})
}

func TestUpdateBalanceAllowance(t *testing.T) {
	client := newTestClobClientWithAuth(t)

	// 注意：这个测试会实际更新余额授权
	t.Run("Basic", func(t *testing.T) {
		amount := 1000.0
		balanceAllowance, err := client.UpdateBalanceAllowance(amount)
		if err != nil {
			t.Fatalf("UpdateBalanceAllowance failed: %v", err)
		}
		if balanceAllowance == nil {
			t.Fatal("UpdateBalanceAllowance returned nil")
		}
		t.Logf("UpdateBalanceAllowance returned: %+v", balanceAllowance)
	})
}

func TestGetNotifications(t *testing.T) {
	client := newTestClobClientWithAuth(t)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		notifications, err := client.GetNotifications(10, 0)
		if err != nil {
			t.Fatalf("GetNotifications failed: %v", err)
		}
		if notifications == nil {
			t.Fatal("GetNotifications returned nil")
		}
		if len(notifications) > 10 {
			t.Errorf("Expected at most 10 notifications, got %d", len(notifications))
		}
		t.Logf("GetNotifications returned %d notifications", len(notifications))
	})
}

func TestDropNotifications(t *testing.T) {
	client := newTestClobClientWithAuth(t)

	// 注意：这个测试需要有效的通知ID
	t.Run("Basic", func(t *testing.T) {
		// 先获取一些通知
		notifications, err := client.GetNotifications(10, 0)
		if err != nil {
			t.Fatalf("GetNotifications failed: %v", err)
		}

		if len(notifications) == 0 {
			t.Skip("Skipping test: No notifications to drop")
		}

		// 删除第一个通知
		notificationIDs := []string{notifications[0].ID}
		err = client.DropNotifications(notificationIDs)
		if err != nil {
			t.Fatalf("DropNotifications failed: %v", err)
		}
		t.Logf("DropNotifications succeeded")
	})

	// 空数组测试
	t.Run("EmptyArray", func(t *testing.T) {
		err := client.DropNotifications([]string{})
		if err != nil {
			t.Fatalf("DropNotifications with empty array failed: %v", err)
		}
	})
}

func TestIsOrderScoring(t *testing.T) {
	client := newTestClobClientWithAuth(t)

	// 注意：这个测试需要有效的订单ID
	t.Run("Basic", func(t *testing.T) {
		// 先获取一些订单
		orders, err := client.GetOrders(nil, nil, nil)
		if err != nil {
			t.Fatalf("GetOrders failed: %v", err)
		}

		if len(orders) == 0 {
			t.Skip("Skipping test: No orders to check")
		}

		// 检查第一个订单是否计分
		isScoring, err := client.IsOrderScoring(orders[0].OrderID)
		if err != nil {
			t.Fatalf("IsOrderScoring failed: %v", err)
		}
		t.Logf("IsOrderScoring returned: %v", isScoring)
	})
}

func TestAreOrdersScoring(t *testing.T) {
	client := newTestClobClientWithAuth(t)

	// 注意：这个测试需要有效的订单ID
	t.Run("Basic", func(t *testing.T) {
		// 先获取一些订单
		orders, err := client.GetOrders(nil, nil, nil)
		if err != nil {
			t.Fatalf("GetOrders failed: %v", err)
		}

		if len(orders) == 0 {
			t.Skip("Skipping test: No orders to check")
		}

		// 检查前两个订单是否计分
		orderIDs := []types.Keccak256{orders[0].OrderID}
		if len(orders) > 1 {
			orderIDs = append(orderIDs, orders[1].OrderID)
		}

		results, err := client.AreOrdersScoring(orderIDs)
		if err != nil {
			t.Fatalf("AreOrdersScoring failed: %v", err)
		}
		if results == nil {
			t.Fatal("AreOrdersScoring returned nil")
		}
		t.Logf("AreOrdersScoring returned: %+v", results)
	})
}

func TestGetAPIKeys(t *testing.T) {
	client := newTestClobClientWithAuth(t)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		apiKeys, err := client.GetAPIKeys()
		if err != nil {
			t.Fatalf("GetAPIKeys failed: %v", err)
		}
		if apiKeys == nil {
			t.Fatal("GetAPIKeys returned nil")
		}
		t.Logf("GetAPIKeys returned %d API keys", len(apiKeys))
	})
}

func TestDeleteAPIKey(t *testing.T) {
	client := newTestClobClientWithAuth(t)

	// 注意：这个测试需要有效的API密钥ID
	t.Run("Basic", func(t *testing.T) {
		// 先获取一些API密钥
		apiKeys, err := client.GetAPIKeys()
		if err != nil {
			t.Fatalf("GetAPIKeys failed: %v", err)
		}

		if len(apiKeys) == 0 {
			t.Skip("Skipping test: No API keys to delete")
		}

		// 删除第一个API密钥（注意：这会实际删除）
		err = client.DeleteAPIKey(apiKeys[0].ID)
		if err != nil {
			t.Fatalf("DeleteAPIKey failed: %v", err)
		}
		t.Logf("DeleteAPIKey succeeded")
	})
}

func TestCreateReadonlyAPIKey(t *testing.T) {
	client := newTestClobClientWithAuth(t)

	// 注意：这个测试会实际创建只读API密钥
	t.Run("Basic", func(t *testing.T) {
		apiKey, err := client.CreateReadonlyAPIKey()
		if err != nil {
			t.Fatalf("CreateReadonlyAPIKey failed: %v", err)
		}
		if apiKey == nil {
			t.Fatal("CreateReadonlyAPIKey returned nil")
		}
		t.Logf("CreateReadonlyAPIKey returned: %+v", apiKey)

		// 清理：删除刚创建的API密钥
		defer func() {
			if apiKey.ID != "" {
				_ = client.DeleteReadonlyAPIKey(apiKey.ID)
			}
		}()
	})
}

func TestGetReadonlyAPIKeys(t *testing.T) {
	client := newTestClobClientWithAuth(t)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		apiKeys, err := client.GetReadonlyAPIKeys()
		if err != nil {
			t.Fatalf("GetReadonlyAPIKeys failed: %v", err)
		}
		if apiKeys == nil {
			t.Fatal("GetReadonlyAPIKeys returned nil")
		}
		t.Logf("GetReadonlyAPIKeys returned %d readonly API keys", len(apiKeys))
	})
}

func TestDeleteReadonlyAPIKey(t *testing.T) {
	client := newTestClobClientWithAuth(t)

	// 注意：这个测试需要有效的只读API密钥ID
	t.Run("Basic", func(t *testing.T) {
		// 先获取一些只读API密钥
		apiKeys, err := client.GetReadonlyAPIKeys()
		if err != nil {
			t.Fatalf("GetReadonlyAPIKeys failed: %v", err)
		}

		if len(apiKeys) == 0 {
			t.Skip("Skipping test: No readonly API keys to delete")
		}

		// 删除第一个只读API密钥（注意：这会实际删除）
		err = client.DeleteReadonlyAPIKey(apiKeys[0].ID)
		if err != nil {
			t.Fatalf("DeleteReadonlyAPIKey failed: %v", err)
		}
		t.Logf("DeleteReadonlyAPIKey succeeded")
	})
}
