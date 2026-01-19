package websocket

import (
	"sync"
	"testing"
	"time"

	"github.com/polymas/go-polymarket-sdk/test"
	"github.com/polymas/go-polymarket-sdk/types"
)

func TestSetOnBookUpdate(t *testing.T) {
	client := NewClient(test.DefaultReconnectDelay)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		called := false
		client.SetOnBookUpdate(func(assetID string, snapshot *types.BookSnapshot) {
			called = true
		})
		// 设置回调不应该出错
		if !called {
			// 回调只有在收到消息时才会被调用
			t.Logf("SetOnBookUpdate succeeded (callback will be called on message)")
		}
	})
}

func TestSetOnOrderUpdate(t *testing.T) {
	client := NewClient(test.DefaultReconnectDelay)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		called := false
		client.SetOnOrderUpdate(func(order *types.OpenOrder) {
			called = true
		})
		// 设置回调不应该出错
		if !called {
			t.Logf("SetOnOrderUpdate succeeded (callback will be called on message)")
		}
	})
}

func TestSetOnTradeUpdate(t *testing.T) {
	client := NewClient(test.DefaultReconnectDelay)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		called := false
		client.SetOnTradeUpdate(func(trade *types.PolygonTrade) {
			called = true
		})
		// 设置回调不应该出错
		if !called {
			t.Logf("SetOnTradeUpdate succeeded (callback will be called on message)")
		}
	})
}

func TestSetAuth(t *testing.T) {
	client := NewClient(test.DefaultReconnectDelay)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		auth := &WebSocketAuth{
			Address:   "0x1234567890123456789012345678901234567890",
			Signature: "0xsignature",
			Timestamp: "1234567890",
			Nonce:     "0",
		}
		client.SetAuth(auth)
		// 设置认证不应该出错
		t.Logf("SetAuth succeeded")
	})
}

func TestStart(t *testing.T) {
	client := NewClient(test.DefaultReconnectDelay)
	config := test.LoadTestConfig()

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		assetIDs := []string{config.TestTokenID}

		// 设置回调
		var receivedUpdate bool
		var mu sync.Mutex
		client.SetOnBookUpdate(func(assetID string, snapshot *types.BookSnapshot) {
			mu.Lock()
			receivedUpdate = true
			mu.Unlock()
			t.Logf("Received book update for asset: %s", assetID)
		})

		err := client.Start(assetIDs)
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		// 等待一段时间以接收消息
		time.Sleep(5 * time.Second)

		// 检查是否运行
		if !client.IsRunning() {
			t.Error("Client should be running after Start")
		}

		// 停止客户端
		client.Stop()

		// 等待停止完成
		time.Sleep(1 * time.Second)

		if client.IsRunning() {
			t.Error("Client should not be running after Stop")
		}

		mu.Lock()
		hasUpdate := receivedUpdate
		mu.Unlock()

		if !hasUpdate {
			t.Logf("No updates received (may be expected if no market activity)")
		}
	})
}

func TestStop(t *testing.T) {
	client := NewClient(test.DefaultReconnectDelay)
	config := test.LoadTestConfig()

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		assetIDs := []string{config.TestTokenID}

		err := client.Start(assetIDs)
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		// 等待连接建立
		time.Sleep(2 * time.Second)

		// 停止客户端
		client.Stop()

		// 等待停止完成
		time.Sleep(1 * time.Second)

		if client.IsRunning() {
			t.Error("Client should not be running after Stop")
		}
	})
}

func TestIsRunning(t *testing.T) {
	client := NewClient(test.DefaultReconnectDelay)

	// 初始状态应该是未运行
	t.Run("InitialState", func(t *testing.T) {
		if client.IsRunning() {
			t.Error("Client should not be running initially")
		}
	})

	config := test.LoadTestConfig()
	if config.TestTokenID == "" {
		t.Skip("Skipping remaining tests: POLY_TEST_TOKEN_ID not set")
	}

	// 启动后应该是运行状态
	t.Run("AfterStart", func(t *testing.T) {
		assetIDs := []string{config.TestTokenID}
		err := client.Start(assetIDs)
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		time.Sleep(2 * time.Second)

		if !client.IsRunning() {
			t.Error("Client should be running after Start")
		}

		client.Stop()
		time.Sleep(1 * time.Second)
	})
}

func TestUpdateSubscription(t *testing.T) {
	client := NewClient(test.DefaultReconnectDelay)
	config := test.LoadTestConfig()

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		assetIDs := []string{config.TestTokenID}

		err := client.Start(assetIDs)
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		// 等待连接建立
		time.Sleep(2 * time.Second)

		// 更新订阅
		newAssetIDs := []string{config.TestTokenID}
		err = client.UpdateSubscription(newAssetIDs)
		if err != nil {
			t.Fatalf("UpdateSubscription failed: %v", err)
		}

		client.Stop()
		time.Sleep(1 * time.Second)
	})
}

func TestSubscribeAssets(t *testing.T) {
	client := NewClient(test.DefaultReconnectDelay)
	config := test.LoadTestConfig()

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		assetIDs := []string{config.TestTokenID}

		err := client.Start(assetIDs)
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		// 等待连接建立
		time.Sleep(2 * time.Second)

		// 订阅额外资产
		additionalAssets := []string{config.TestTokenID}
		err = client.SubscribeAssets(additionalAssets)
		if err != nil {
			t.Fatalf("SubscribeAssets failed: %v", err)
		}

		client.Stop()
		time.Sleep(1 * time.Second)
	})
}

func TestUnsubscribeAssets(t *testing.T) {
	client := NewClient(test.DefaultReconnectDelay)
	config := test.LoadTestConfig()

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		assetIDs := []string{config.TestTokenID}

		err := client.Start(assetIDs)
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		// 等待连接建立
		time.Sleep(2 * time.Second)

		// 取消订阅
		unsubscribeAssets := []string{config.TestTokenID}
		err = client.UnsubscribeAssets(unsubscribeAssets)
		if err != nil {
			t.Fatalf("UnsubscribeAssets failed: %v", err)
		}

		client.Stop()
		time.Sleep(1 * time.Second)
	})
}

func TestStartUserChannel(t *testing.T) {
	client := NewClient(test.DefaultReconnectDelay)
	config := test.LoadTestConfig()

	test.SkipIfNoAuth(t, config)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		// 设置认证（用户频道需要认证）
		auth := &WebSocketAuth{
			Address:   string(test.GetTestUserAddress(config)),
			Signature: "0xsignature",
			Timestamp: "1234567890",
			Nonce:     "0",
		}
		client.SetAuth(auth)

		// 设置订单更新回调
		var receivedOrder bool
		var mu sync.Mutex
		client.SetOnOrderUpdate(func(order *types.OpenOrder) {
			mu.Lock()
			receivedOrder = true
			mu.Unlock()
			t.Logf("Received order update: %+v", order)
		})

		err := client.StartUserChannel()
		if err != nil {
			t.Fatalf("StartUserChannel failed: %v", err)
		}

		// 等待一段时间以接收消息
		time.Sleep(5 * time.Second)

		// 停止用户频道
		client.StopUserChannel()

		// 等待停止完成
		time.Sleep(1 * time.Second)

		mu.Lock()
		hasOrder := receivedOrder
		mu.Unlock()

		if !hasOrder {
			t.Logf("No order updates received (may be expected if no orders)")
		}
	})
}

func TestStopUserChannel(t *testing.T) {
	client := NewClient(test.DefaultReconnectDelay)
	config := test.LoadTestConfig()

	test.SkipIfNoAuth(t, config)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		// 设置认证
		auth := &WebSocketAuth{
			Address:   string(test.GetTestUserAddress(config)),
			Signature: "0xsignature",
			Timestamp: "1234567890",
			Nonce:     "0",
		}
		client.SetAuth(auth)

		err := client.StartUserChannel()
		if err != nil {
			t.Fatalf("StartUserChannel failed: %v", err)
		}

		// 等待连接建立
		time.Sleep(2 * time.Second)

		// 停止用户频道
		client.StopUserChannel()

		// 等待停止完成
		time.Sleep(1 * time.Second)

		t.Logf("StopUserChannel succeeded")
	})
}
