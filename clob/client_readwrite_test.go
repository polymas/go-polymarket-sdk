package clob

import (
	"testing"
	"time"

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
	config := test.LoadTestConfig()

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

	// 测试所有参数组合
	if config.TestTokenID != "" && config.TestConditionID != "" {
		t.Run("WithAllParams", func(t *testing.T) {
			conditionID := config.TestConditionID
			tokenID := config.TestTokenID
			orders, err := client.GetOrders(nil, &conditionID, &tokenID)
			if err != nil {
				t.Fatalf("GetOrders with all params failed: %v", err)
			}
			if orders == nil {
				t.Fatal("GetOrders returned nil")
			}
			t.Logf("GetOrders with all params returned %d orders", len(orders))
		})
	}

	// 测试无效conditionID
	t.Run("InvalidConditionID", func(t *testing.T) {
		invalidConditionID := types.Keccak256("invalid-condition-id")
		orders, err := client.GetOrders(nil, &invalidConditionID, nil)
		// 可能返回空结果或错误，都是可以接受的
		if err != nil {
			t.Logf("GetOrders with invalid conditionID returned error (expected): %v", err)
		} else if orders != nil {
			t.Logf("GetOrders with invalid conditionID returned %d orders (may be empty)", len(orders))
		}
	})

	// 测试无效tokenID
	if config.TestTokenID != "" {
		t.Run("InvalidTokenID", func(t *testing.T) {
			invalidTokenID := "invalid-token-id"
			orders, err := client.GetOrders(nil, nil, &invalidTokenID)
			// 可能返回空结果或错误，都是可以接受的
			if err != nil {
				t.Logf("GetOrders with invalid tokenID returned error (expected): %v", err)
			} else if orders != nil {
				t.Logf("GetOrders with invalid tokenID returned %d orders (may be empty)", len(orders))
			}
		})
	}

	// 测试空结果集（当没有订单时）
	t.Run("EmptyResultSet", func(t *testing.T) {
		// 使用一个不太可能有订单的条件ID
		unlikelyConditionID := types.Keccak256("0x0000000000000000000000000000000000000000000000000000000000000000")
		orders, err := client.GetOrders(nil, &unlikelyConditionID, nil)
		if err != nil {
			t.Logf("GetOrders with unlikely conditionID returned error: %v", err)
		} else if orders != nil {
			if len(orders) == 0 {
				t.Logf("GetOrders returned empty result set (expected)")
			} else {
				t.Logf("GetOrders returned %d orders", len(orders))
			}
		}
	})

	// 测试分页功能（通过检查是否有多个订单来验证分页）
	t.Run("Pagination", func(t *testing.T) {
		orders, err := client.GetOrders(nil, nil, nil)
		if err != nil {
			t.Fatalf("GetOrders failed: %v", err)
		}
		if orders != nil {
			t.Logf("GetOrders pagination test: returned %d orders (pagination handled internally)", len(orders))
		}
	})
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

	// 测试空数组
	t.Run("EmptyArray", func(t *testing.T) {
		responses, err := client.CreateAndPostOrders([]types.OrderArgs{}, []types.OrderType{})
		if err != nil {
			t.Fatalf("CreateAndPostOrders with empty array failed: %v", err)
		}
		if responses == nil {
			t.Fatal("CreateAndPostOrders returned nil")
		}
		if len(responses) != 0 {
			t.Errorf("Expected 0 responses, got %d", len(responses))
		}
	})

	// 测试orderArgsList和orderTypes长度不匹配
	t.Run("LengthMismatch", func(t *testing.T) {
		orderArgs := []types.OrderArgs{
			{
				TokenID: config.TestTokenID,
				Side:    types.OrderSideBUY,
				Price:   0.5,
				Size:    10.0,
			},
		}
		orderTypes := []types.OrderType{types.OrderTypeGTC, types.OrderTypeIOC} // 长度不匹配

		_, err := client.CreateAndPostOrders(orderArgs, orderTypes)
		if err == nil {
			t.Error("Expected error for length mismatch")
		}
		t.Logf("Length mismatch error (expected): %v", err)
	})

	// 测试不同OrderType
	t.Run("DifferentOrderTypes", func(t *testing.T) {
		orderArgs := []types.OrderArgs{
			{
				TokenID: config.TestTokenID,
				Side:    types.OrderSideBUY,
				Price:   0.5,
				Size:    10.0,
			},
		}

		orderTypes := []types.OrderType{types.OrderTypeIOC}
		responses, err := client.CreateAndPostOrders(orderArgs, orderTypes)
		if err != nil {
			t.Logf("CreateAndPostOrders with IOC failed (may be expected): %v", err)
		} else if responses != nil && len(responses) > 0 {
			t.Logf("CreateAndPostOrders with IOC succeeded")
		}

		orderTypes = []types.OrderType{types.OrderTypeFOK}
		responses, err = client.CreateAndPostOrders(orderArgs, orderTypes)
		if err != nil {
			t.Logf("CreateAndPostOrders with FOK failed (may be expected): %v", err)
		} else if responses != nil && len(responses) > 0 {
			t.Logf("CreateAndPostOrders with FOK succeeded")
		}
	})

	// 测试价格超出范围的情况
	t.Run("PriceOutOfRange", func(t *testing.T) {
		testCases := []struct {
			name  string
			price float64
		}{
			{"PriceTooLow", 0.0001},
			{"PriceTooHigh", 0.9999},
			{"PriceZero", 0.0},
			{"PriceOne", 1.0},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				orderArgs := []types.OrderArgs{
					{
						TokenID: config.TestTokenID,
						Side:    types.OrderSideBUY,
						Price:   tc.price,
						Size:    10.0,
					},
				}
				orderTypes := []types.OrderType{types.OrderTypeGTC}

				_, err := client.CreateAndPostOrders(orderArgs, orderTypes)
				if err != nil {
					t.Logf("CreateAndPostOrders with price %f returned error (expected): %v", tc.price, err)
				} else {
					t.Logf("CreateAndPostOrders with price %f succeeded (may be adjusted)", tc.price)
				}
			})
		}
	})

	// 测试批量订单（>15个订单的分批处理）
	t.Run("BatchOrders", func(t *testing.T) {
		// 创建20个订单来测试分批处理
		orderArgs := make([]types.OrderArgs, 20)
		orderTypes := make([]types.OrderType, 20)

		for i := 0; i < 20; i++ {
			orderArgs[i] = types.OrderArgs{
				TokenID: config.TestTokenID,
				Side:    types.OrderSideBUY,
				Price:   0.5 + float64(i)*0.01, // 不同的价格
				Size:    10.0,
			}
			orderTypes[i] = types.OrderTypeGTC
		}

		responses, err := client.CreateAndPostOrders(orderArgs, orderTypes)
		if err != nil {
			t.Logf("CreateAndPostOrders with batch failed (may be expected): %v", err)
		} else if responses != nil {
			if len(responses) != 20 {
				t.Errorf("Expected 20 responses, got %d", len(responses))
			}
			t.Logf("CreateAndPostOrders with batch returned %d responses", len(responses))
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

	// 测试所有OrderType
	t.Run("AllOrderTypes", func(t *testing.T) {
		orderArgs := types.OrderArgs{
			TokenID: config.TestTokenID,
			Side:    types.OrderSideBUY,
			Price:   0.5,
			Size:    10.0,
		}

		orderTypes := []types.OrderType{
			types.OrderTypeGTC,
			types.OrderTypeIOC,
			types.OrderTypeFOK,
		}

		for _, orderType := range orderTypes {
			t.Run(string(orderType), func(t *testing.T) {
				response, err := client.PostOrder(orderArgs, orderType)
				if err != nil {
					t.Logf("PostOrder with %s failed (may be expected): %v", orderType, err)
				} else if response != nil {
					if response.ErrorMsg != "" {
						t.Logf("PostOrder with %s returned error: %s", orderType, response.ErrorMsg)
					} else {
						t.Logf("PostOrder with %s succeeded", orderType)
					}
				}
			})
		}
	})

	// 测试价格边界值
	t.Run("PriceBoundaries", func(t *testing.T) {
		testCases := []struct {
			name  string
			price float64
		}{
			{"MinPrice", 0.001},
			{"MaxPrice", 0.999},
			{"MidPrice", 0.5},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				orderArgs := types.OrderArgs{
					TokenID: config.TestTokenID,
					Side:    types.OrderSideBUY,
					Price:   tc.price,
					Size:    10.0,
				}

				response, err := client.PostOrder(orderArgs, types.OrderTypeGTC)
				if err != nil {
					t.Logf("PostOrder with price %f failed (may be expected): %v", tc.price, err)
				} else if response != nil {
					if response.ErrorMsg != "" {
						t.Logf("PostOrder with price %f returned error: %s", tc.price, response.ErrorMsg)
					} else {
						t.Logf("PostOrder with price %f succeeded", tc.price)
					}
				}
			})
		}
	})

	// 测试size边界值
	t.Run("SizeBoundaries", func(t *testing.T) {
		testCases := []struct {
			name string
			size float64
		}{
			{"MinSize", 5.0},
			{"SmallSize", 10.0},
			{"LargeSize", 1000.0},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				orderArgs := types.OrderArgs{
					TokenID: config.TestTokenID,
					Side:    types.OrderSideBUY,
					Price:   0.5,
					Size:    tc.size,
				}

				response, err := client.PostOrder(orderArgs, types.OrderTypeGTC)
				if err != nil {
					t.Logf("PostOrder with size %f failed (may be expected): %v", tc.size, err)
				} else if response != nil {
					if response.ErrorMsg != "" {
						t.Logf("PostOrder with size %f returned error: %s", tc.size, response.ErrorMsg)
					} else {
						t.Logf("PostOrder with size %f succeeded", tc.size)
					}
				}
			})
		}
	})

	// 测试无效tokenID
	t.Run("InvalidTokenID", func(t *testing.T) {
		orderArgs := types.OrderArgs{
			TokenID: "invalid-token-id",
			Side:    types.OrderSideBUY,
			Price:   0.5,
			Size:    10.0,
		}

		response, err := client.PostOrder(orderArgs, types.OrderTypeGTC)
		if err != nil {
			t.Logf("PostOrder with invalid tokenID returned error (expected): %v", err)
		} else if response != nil && response.ErrorMsg != "" {
			t.Logf("PostOrder with invalid tokenID returned error message: %s", response.ErrorMsg)
		} else {
			t.Error("Expected error for invalid tokenID")
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

	// 测试批量取消（多个订单）
	t.Run("BatchCancel", func(t *testing.T) {
		orders, err := client.GetOrders(nil, nil, nil)
		if err != nil {
			t.Fatalf("GetOrders failed: %v", err)
		}

		if len(orders) < 2 {
			t.Skip("Skipping test: Need at least 2 orders for batch cancel")
		}

		// 取消前两个订单
		orderIDs := []types.Keccak256{orders[0].OrderID, orders[1].OrderID}
		response, err := client.CancelOrders(orderIDs)
		if err != nil {
			t.Fatalf("CancelOrders with batch failed: %v", err)
		}
		if response == nil {
			t.Fatal("CancelOrders returned nil")
		}
		t.Logf("CancelOrders batch response: %+v", response)
	})

	// 测试无效订单ID
	t.Run("InvalidOrderID", func(t *testing.T) {
		invalidOrderID := types.Keccak256("invalid-order-id")
		response, err := client.CancelOrders([]types.Keccak256{invalidOrderID})
		if err != nil {
			t.Logf("CancelOrders with invalid orderID returned error (expected): %v", err)
		} else if response != nil {
			t.Logf("CancelOrders with invalid orderID response: %+v", response)
		}
	})

	// 测试已取消的订单（先取消一个订单，然后再尝试取消它）
	t.Run("AlreadyCanceled", func(t *testing.T) {
		orders, err := client.GetOrders(nil, nil, nil)
		if err != nil {
			t.Fatalf("GetOrders failed: %v", err)
		}

		if len(orders) == 0 {
			t.Skip("Skipping test: No orders to cancel")
		}

		orderID := orders[0].OrderID
		// 第一次取消
		_, err = client.CancelOrders([]types.Keccak256{orderID})
		if err != nil {
			t.Logf("First cancel failed: %v", err)
			return
		}

		// 等待一下确保订单已取消
		time.Sleep(1 * time.Second)

		// 再次尝试取消同一个订单
		response, err := client.CancelOrders([]types.Keccak256{orderID})
		if err != nil {
			t.Logf("CancelOrders with already canceled order returned error (expected): %v", err)
		} else if response != nil {
			t.Logf("CancelOrders with already canceled order response: %+v", response)
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

	// 测试无效订单ID
	t.Run("InvalidOrderID", func(t *testing.T) {
		invalidOrderID := types.Keccak256("invalid-order-id")
		response, err := client.CancelOrder(invalidOrderID)
		if err != nil {
			t.Logf("CancelOrder with invalid orderID returned error (expected): %v", err)
		} else if response != nil {
			t.Logf("CancelOrder with invalid orderID response: %+v", response)
		}
	})

	// 测试已取消的订单
	t.Run("AlreadyCanceled", func(t *testing.T) {
		orders, err := client.GetOrders(nil, nil, nil)
		if err != nil {
			t.Fatalf("GetOrders failed: %v", err)
		}

		if len(orders) == 0 {
			t.Skip("Skipping test: No orders to cancel")
		}

		orderID := orders[0].OrderID
		// 第一次取消
		_, err = client.CancelOrder(orderID)
		if err != nil {
			t.Logf("First cancel failed: %v", err)
			return
		}

		// 等待一下确保订单已取消
		time.Sleep(1 * time.Second)

		// 再次尝试取消同一个订单
		response, err := client.CancelOrder(orderID)
		if err != nil {
			t.Logf("CancelOrder with already canceled order returned error (expected): %v", err)
		} else if response != nil {
			t.Logf("CancelOrder with already canceled order response: %+v", response)
		}
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

	// 测试无效conditionID
	t.Run("InvalidConditionID", func(t *testing.T) {
		invalidConditionID := types.Keccak256("invalid-condition-id")
		response, err := client.CancelMarketOrders(invalidConditionID)
		if err != nil {
			t.Logf("CancelMarketOrders with invalid conditionID returned error (expected): %v", err)
		} else if response != nil {
			t.Logf("CancelMarketOrders with invalid conditionID response: %+v", response)
		}
	})

	// 测试该市场无订单的情况
	t.Run("NoOrdersInMarket", func(t *testing.T) {
		// 使用一个不太可能有订单的条件ID
		unlikelyConditionID := types.Keccak256("0x0000000000000000000000000000000000000000000000000000000000000000")
		response, err := client.CancelMarketOrders(unlikelyConditionID)
		if err != nil {
			t.Logf("CancelMarketOrders with no orders returned error: %v", err)
		} else if response != nil {
			if len(response.Canceled) == 0 {
				t.Logf("CancelMarketOrders with no orders returned empty canceled list (expected)")
			} else {
				t.Logf("CancelMarketOrders with no orders returned %d canceled orders", len(response.Canceled))
			}
		}
	})
}

func TestGetUSDCBalance(t *testing.T) {
	client := newTestClobClientWithAuth(t)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		balance, err := client.GetUSDCBalance()
		if err != nil {
			t.Fatalf("GetUSDCBalance failed: %v", err)
		}
		if balance < 0 {
			t.Errorf("Expected non-negative balance, got %f", balance)
		}
		t.Logf("GetUSDCBalance returned: %f", balance)
	})

	// 测试余额为0的情况（如果账户余额为0）
	t.Run("ZeroBalance", func(t *testing.T) {
		balance, err := client.GetUSDCBalance()
		if err != nil {
			t.Fatalf("GetUSDCBalance failed: %v", err)
		}
		if balance == 0 {
			t.Logf("GetUSDCBalance returned zero balance (acceptable)")
		} else {
			t.Logf("GetUSDCBalance returned non-zero balance: %f", balance)
		}
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

	// 测试授权为0的情况
	t.Run("ZeroAllowance", func(t *testing.T) {
		balanceAllowance, err := client.GetBalanceAllowance()
		if err != nil {
			t.Fatalf("GetBalanceAllowance failed: %v", err)
		}
		if balanceAllowance != nil {
			t.Logf("GetBalanceAllowance returned: %+v (may have zero allowance)", balanceAllowance)
		}
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

	// 测试0金额
	t.Run("ZeroAmount", func(t *testing.T) {
		balanceAllowance, err := client.UpdateBalanceAllowance(0.0)
		if err != nil {
			t.Logf("UpdateBalanceAllowance with zero amount returned error (may be expected): %v", err)
		} else if balanceAllowance != nil {
			t.Logf("UpdateBalanceAllowance with zero amount succeeded: %+v", balanceAllowance)
		}
	})

	// 测试负数金额
	t.Run("NegativeAmount", func(t *testing.T) {
		_, err := client.UpdateBalanceAllowance(-100.0)
		if err == nil {
			t.Error("Expected error for negative amount")
		} else {
			t.Logf("UpdateBalanceAllowance with negative amount returned error (expected): %v", err)
		}
	})

	// 测试极大金额
	t.Run("LargeAmount", func(t *testing.T) {
		largeAmount := 1e15 // 非常大的金额
		balanceAllowance, err := client.UpdateBalanceAllowance(largeAmount)
		if err != nil {
			t.Logf("UpdateBalanceAllowance with large amount returned error (may be expected): %v", err)
		} else if balanceAllowance != nil {
			t.Logf("UpdateBalanceAllowance with large amount succeeded: %+v", balanceAllowance)
		}
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
				notifications, err := client.GetNotifications(tc.limit, 0)
				if err != nil {
					t.Logf("GetNotifications with limit %d returned error (may be expected): %v", tc.limit, err)
				} else if notifications != nil {
					t.Logf("GetNotifications with limit %d returned %d notifications", tc.limit, len(notifications))
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
				notifications, err := client.GetNotifications(10, tc.offset)
				if err != nil {
					t.Logf("GetNotifications with offset %d returned error (may be expected): %v", tc.offset, err)
				} else if notifications != nil {
					t.Logf("GetNotifications with offset %d returned %d notifications", tc.offset, len(notifications))
				}
			})
		}
	})

	// 测试分页功能
	t.Run("Pagination", func(t *testing.T) {
		// 获取第一页
		page1, err := client.GetNotifications(10, 0)
		if err != nil {
			t.Fatalf("GetNotifications page 1 failed: %v", err)
		}

		// 获取第二页
		page2, err := client.GetNotifications(10, 10)
		if err != nil {
			t.Fatalf("GetNotifications page 2 failed: %v", err)
		}

		t.Logf("Pagination test: page 1 returned %d notifications, page 2 returned %d notifications", len(page1), len(page2))
	})

	// 测试无通知的情况（使用很大的offset）
	t.Run("NoNotifications", func(t *testing.T) {
		notifications, err := client.GetNotifications(10, 999999)
		if err != nil {
			t.Logf("GetNotifications with large offset returned error: %v", err)
		} else if notifications != nil {
			if len(notifications) == 0 {
				t.Logf("GetNotifications with large offset returned empty result (expected)")
			} else {
				t.Logf("GetNotifications with large offset returned %d notifications", len(notifications))
			}
		}
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

	// 测试无效通知ID
	t.Run("InvalidNotificationID", func(t *testing.T) {
		err := client.DropNotifications([]string{"invalid-notification-id"})
		if err != nil {
			t.Logf("DropNotifications with invalid ID returned error (expected): %v", err)
		} else {
			t.Logf("DropNotifications with invalid ID succeeded (may be acceptable)")
		}
	})

	// 测试已删除的通知（先删除一个，然后再尝试删除它）
	t.Run("AlreadyDeleted", func(t *testing.T) {
		notifications, err := client.GetNotifications(10, 0)
		if err != nil {
			t.Fatalf("GetNotifications failed: %v", err)
		}

		if len(notifications) == 0 {
			t.Skip("Skipping test: No notifications to drop")
		}

		notificationID := notifications[0].ID
		// 第一次删除
		err = client.DropNotifications([]string{notificationID})
		if err != nil {
			t.Logf("First drop failed: %v", err)
			return
		}

		// 等待一下确保通知已删除
		time.Sleep(1 * time.Second)

		// 再次尝试删除同一个通知
		err = client.DropNotifications([]string{notificationID})
		if err != nil {
			t.Logf("DropNotifications with already deleted notification returned error (expected): %v", err)
		} else {
			t.Logf("DropNotifications with already deleted notification succeeded (may be acceptable)")
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

	// 测试无效订单ID
	t.Run("InvalidOrderID", func(t *testing.T) {
		invalidOrderID := types.Keccak256("invalid-order-id")
		_, err := client.IsOrderScoring(invalidOrderID)
		if err != nil {
			t.Logf("IsOrderScoring with invalid orderID returned error (expected): %v", err)
		} else {
			t.Logf("IsOrderScoring with invalid orderID succeeded (may return false)")
		}
	})

	// 测试不存在的订单
	t.Run("NonExistentOrder", func(t *testing.T) {
		nonExistentOrderID := types.Keccak256("0x0000000000000000000000000000000000000000000000000000000000000000")
		isScoring, err := client.IsOrderScoring(nonExistentOrderID)
		if err != nil {
			t.Logf("IsOrderScoring with non-existent order returned error: %v", err)
		} else {
			t.Logf("IsOrderScoring with non-existent order returned: %v (may be false)", isScoring)
		}
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

	// 测试空数组
	t.Run("EmptyArray", func(t *testing.T) {
		results, err := client.AreOrdersScoring([]types.Keccak256{})
		if err != nil {
			t.Fatalf("AreOrdersScoring with empty array failed: %v", err)
		}
		if results == nil {
			t.Fatal("AreOrdersScoring returned nil")
		}
		if len(results) != 0 {
			t.Errorf("Expected empty map, got %d entries", len(results))
		}
		t.Logf("AreOrdersScoring with empty array returned empty map (expected)")
	})

	// 测试大量订单（>100个）
	t.Run("LargeOrderList", func(t *testing.T) {
		orders, err := client.GetOrders(nil, nil, nil)
		if err != nil {
			t.Fatalf("GetOrders failed: %v", err)
		}

		if len(orders) < 5 {
			t.Skip("Skipping test: Need at least 5 orders for large list test")
		}

		// 创建包含重复订单ID的列表（模拟大量订单）
		orderIDs := make([]types.Keccak256, 150)
		for i := 0; i < 150; i++ {
			orderIDs[i] = orders[i%len(orders)].OrderID
		}

		results, err := client.AreOrdersScoring(orderIDs)
		if err != nil {
			t.Fatalf("AreOrdersScoring with large list failed: %v", err)
		}
		if results == nil {
			t.Fatal("AreOrdersScoring returned nil")
		}
		t.Logf("AreOrdersScoring with large list returned %d results", len(results))
	})

	// 测试部分订单无效的情况
	t.Run("PartialInvalidOrders", func(t *testing.T) {
		orders, err := client.GetOrders(nil, nil, nil)
		if err != nil {
			t.Fatalf("GetOrders failed: %v", err)
		}

		if len(orders) == 0 {
			t.Skip("Skipping test: No orders to check")
		}

		orderIDs := []types.Keccak256{
			orders[0].OrderID,
			types.Keccak256("invalid-order-id"),
		}

		results, err := client.AreOrdersScoring(orderIDs)
		if err != nil {
			t.Logf("AreOrdersScoring with partial invalid orders returned error: %v", err)
		} else if results != nil {
			t.Logf("AreOrdersScoring with partial invalid orders returned %d results", len(results))
		}
	})

	// 测试混合计分状态（如果可能）
	t.Run("MixedScoringStates", func(t *testing.T) {
		orders, err := client.GetOrders(nil, nil, nil)
		if err != nil {
			t.Fatalf("GetOrders failed: %v", err)
		}

		if len(orders) < 2 {
			t.Skip("Skipping test: Need at least 2 orders")
		}

		orderIDs := []types.Keccak256{orders[0].OrderID, orders[1].OrderID}
		results, err := client.AreOrdersScoring(orderIDs)
		if err != nil {
			t.Fatalf("AreOrdersScoring failed: %v", err)
		}
		if results != nil {
			scoringCount := 0
			nonScoringCount := 0
			for _, isScoring := range results {
				if isScoring {
					scoringCount++
				} else {
					nonScoringCount++
				}
			}
			t.Logf("AreOrdersScoring mixed states: %d scoring, %d non-scoring", scoringCount, nonScoringCount)
		}
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

	// 测试无API密钥的情况（如果返回空数组）
	t.Run("NoAPIKeys", func(t *testing.T) {
		apiKeys, err := client.GetAPIKeys()
		if err != nil {
			t.Fatalf("GetAPIKeys failed: %v", err)
		}
		if apiKeys != nil {
			if len(apiKeys) == 0 {
				t.Logf("GetAPIKeys returned empty array (no API keys)")
			} else {
				t.Logf("GetAPIKeys returned %d API keys", len(apiKeys))
			}
		}
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

	// 测试无效keyID
	t.Run("InvalidKeyID", func(t *testing.T) {
		err := client.DeleteAPIKey("invalid-key-id")
		if err != nil {
			t.Logf("DeleteAPIKey with invalid keyID returned error (expected): %v", err)
		} else {
			t.Logf("DeleteAPIKey with invalid keyID succeeded (may be acceptable)")
		}
	})

	// 测试删除不存在的密钥
	t.Run("NonExistentKey", func(t *testing.T) {
		nonExistentKeyID := "00000000-0000-0000-0000-000000000000"
		err := client.DeleteAPIKey(nonExistentKeyID)
		if err != nil {
			t.Logf("DeleteAPIKey with non-existent key returned error (expected): %v", err)
		} else {
			t.Logf("DeleteAPIKey with non-existent key succeeded (may be acceptable)")
		}
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

	// 测试创建数量限制（尝试创建多个密钥）
	t.Run("CreateMultipleKeys", func(t *testing.T) {
		var createdKeys []string
		defer func() {
			// 清理：删除所有创建的密钥
			for _, keyID := range createdKeys {
				_ = client.DeleteReadonlyAPIKey(keyID)
			}
		}()

		// 尝试创建3个密钥
		for i := 0; i < 3; i++ {
			apiKey, err := client.CreateReadonlyAPIKey()
			if err != nil {
				t.Logf("CreateReadonlyAPIKey attempt %d failed (may hit limit): %v", i+1, err)
				break
			}
			if apiKey != nil && apiKey.ID != "" {
				createdKeys = append(createdKeys, apiKey.ID)
				t.Logf("CreateReadonlyAPIKey attempt %d succeeded", i+1)
			}
		}
		t.Logf("Created %d readonly API keys", len(createdKeys))
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

	// 测试无只读密钥的情况
	t.Run("NoReadonlyKeys", func(t *testing.T) {
		apiKeys, err := client.GetReadonlyAPIKeys()
		if err != nil {
			t.Fatalf("GetReadonlyAPIKeys failed: %v", err)
		}
		if apiKeys != nil {
			if len(apiKeys) == 0 {
				t.Logf("GetReadonlyAPIKeys returned empty array (no readonly API keys)")
			} else {
				t.Logf("GetReadonlyAPIKeys returned %d readonly API keys", len(apiKeys))
			}
		}
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

	// 测试无效keyID
	t.Run("InvalidKeyID", func(t *testing.T) {
		err := client.DeleteReadonlyAPIKey("invalid-key-id")
		if err != nil {
			t.Logf("DeleteReadonlyAPIKey with invalid keyID returned error (expected): %v", err)
		} else {
			t.Logf("DeleteReadonlyAPIKey with invalid keyID succeeded (may be acceptable)")
		}
	})

	// 测试删除不存在的密钥
	t.Run("NonExistentKey", func(t *testing.T) {
		nonExistentKeyID := "00000000-0000-0000-0000-000000000000"
		err := client.DeleteReadonlyAPIKey(nonExistentKeyID)
		if err != nil {
			t.Logf("DeleteReadonlyAPIKey with non-existent key returned error (expected): %v", err)
		} else {
			t.Logf("DeleteReadonlyAPIKey with non-existent key succeeded (may be acceptable)")
		}
	})
}
