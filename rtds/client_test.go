package rtds

import (
	"sync"
	"testing"
	"time"

	"github.com/polymas/go-polymarket-sdk/test"
	"github.com/polymas/go-polymarket-sdk/types"
)

func TestSetOnPriceUpdate(t *testing.T) {
	client := NewClient(test.DefaultReconnectDelay)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		called := false
		client.SetOnPriceUpdate(func(update *types.EncryptedPriceUpdate) {
			called = true
		})
		// 设置回调不应该出错
		if !called {
			t.Logf("SetOnPriceUpdate succeeded (callback will be called on message)")
		}
	})

	// 测试nil回调
	t.Run("NilCallback", func(t *testing.T) {
		client.SetOnPriceUpdate(nil)
		// 设置nil回调不应该panic
		t.Logf("SetOnPriceUpdate with nil callback succeeded")
	})
}

func TestSetOnCommentUpdate(t *testing.T) {
	client := NewClient(test.DefaultReconnectDelay)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		called := false
		client.SetOnCommentUpdate(func(comment *types.Comment) {
			called = true
		})
		// 设置回调不应该出错
		if !called {
			t.Logf("SetOnCommentUpdate succeeded (callback will be called on message)")
		}
	})

	// 测试nil回调
	t.Run("NilCallback", func(t *testing.T) {
		client.SetOnCommentUpdate(nil)
		// 设置nil回调不应该panic
		t.Logf("SetOnCommentUpdate with nil callback succeeded")
	})
}

func TestSetAuth(t *testing.T) {
	client := NewClient(test.DefaultReconnectDelay)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		auth := &RTDSAuth{
			Token: "test-token",
		}
		client.SetAuth(auth)
		// 设置认证不应该出错
		t.Logf("SetAuth succeeded")
	})
}

func TestStart(t *testing.T) {
	client := NewClient(test.DefaultReconnectDelay)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		// 设置回调
		var receivedPriceUpdate bool
		var receivedCommentUpdate bool
		var mu sync.Mutex
		client.SetOnPriceUpdate(func(update *types.EncryptedPriceUpdate) {
			mu.Lock()
			receivedPriceUpdate = true
			mu.Unlock()
			t.Logf("Received price update: %+v", update)
		})
		client.SetOnCommentUpdate(func(comment *types.Comment) {
			mu.Lock()
			receivedCommentUpdate = true
			mu.Unlock()
			t.Logf("Received comment update: %+v", comment)
		})

		err := client.Start()
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
		hasPriceUpdate := receivedPriceUpdate
		hasCommentUpdate := receivedCommentUpdate
		mu.Unlock()

		if !hasPriceUpdate && !hasCommentUpdate {
			t.Logf("No updates received (may be expected if no activity)")
		}
	})

	// 测试重复启动
	t.Run("DuplicateStart", func(t *testing.T) {
		err := client.Start()
		if err != nil {
			t.Fatalf("First Start failed: %v", err)
		}
		defer client.Stop()

		time.Sleep(1 * time.Second)

		// 尝试再次启动
		err = client.Start()
		if err != nil {
			t.Logf("Duplicate Start returned error (expected): %v", err)
		} else {
			t.Error("Expected error for duplicate Start")
		}
	})
}

func TestStop(t *testing.T) {
	client := NewClient(test.DefaultReconnectDelay)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		err := client.Start()
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

	// 启动后应该是运行状态
	t.Run("AfterStart", func(t *testing.T) {
		err := client.Start()
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

func TestSubscribePrices(t *testing.T) {
	client := NewClient(test.DefaultReconnectDelay)
	config := test.LoadTestConfig()

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		err := client.Start()
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		// 等待连接建立
		time.Sleep(2 * time.Second)

		// 订阅价格
		tokenIDs := []string{config.TestTokenID}
		err = client.SubscribePrices(tokenIDs)
		if err != nil {
			t.Fatalf("SubscribePrices failed: %v", err)
		}

		// 等待一段时间以接收消息
		time.Sleep(3 * time.Second)

		client.Stop()
		time.Sleep(1 * time.Second)
	})

	// 测试未启动时调用
	t.Run("BeforeStart", func(t *testing.T) {
		tokenIDs := []string{config.TestTokenID}
		err := client.SubscribePrices(tokenIDs)
		if err != nil {
			t.Logf("SubscribePrices before Start returned error (expected): %v", err)
		} else {
			t.Logf("SubscribePrices before Start succeeded (may queue subscription)")
		}
	})

	// 测试空tokenIDs
	t.Run("EmptyTokenIDs", func(t *testing.T) {
		err := client.Start()
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		defer client.Stop()

		time.Sleep(2 * time.Second)

		err = client.SubscribePrices([]string{})
		if err != nil {
			t.Logf("SubscribePrices with empty tokenIDs returned error (expected): %v", err)
		} else {
			t.Logf("SubscribePrices with empty tokenIDs succeeded")
		}
	})

	// 测试大量tokenIDs
	t.Run("LargeTokenIDs", func(t *testing.T) {
		err := client.Start()
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		defer client.Stop()

		time.Sleep(2 * time.Second)

		tokenIDs := make([]string, 100)
		for i := 0; i < 100; i++ {
			tokenIDs[i] = config.TestTokenID
		}
		err = client.SubscribePrices(tokenIDs)
		if err != nil {
			t.Logf("SubscribePrices with large tokenIDs returned error: %v", err)
		} else {
			t.Logf("SubscribePrices with large tokenIDs succeeded")
		}
	})

	// 测试无效tokenID
	t.Run("InvalidTokenID", func(t *testing.T) {
		err := client.Start()
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		defer client.Stop()

		time.Sleep(2 * time.Second)

		err = client.SubscribePrices([]string{"invalid-token-id"})
		if err != nil {
			t.Logf("SubscribePrices with invalid tokenID returned error: %v", err)
		} else {
			t.Logf("SubscribePrices with invalid tokenID succeeded (may accept but return no data)")
		}
	})
}

func TestUnsubscribePrices(t *testing.T) {
	client := NewClient(test.DefaultReconnectDelay)
	config := test.LoadTestConfig()

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		err := client.Start()
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		// 等待连接建立
		time.Sleep(2 * time.Second)

		// 先订阅
		tokenIDs := []string{config.TestTokenID}
		err = client.SubscribePrices(tokenIDs)
		if err != nil {
			t.Fatalf("SubscribePrices failed: %v", err)
		}

		time.Sleep(1 * time.Second)

		// 取消订阅
		err = client.UnsubscribePrices(tokenIDs)
		if err != nil {
			t.Fatalf("UnsubscribePrices failed: %v", err)
		}

		client.Stop()
		time.Sleep(1 * time.Second)
	})
}

func TestSubscribeComments(t *testing.T) {
	client := NewClient(test.DefaultReconnectDelay)
	config := test.LoadTestConfig()

	if config.TestMarketID == "" {
		t.Skip("Skipping test: POLY_TEST_MARKET_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		err := client.Start()
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		// 等待连接建立
		time.Sleep(2 * time.Second)

		// 订阅评论
		marketIDs := []string{config.TestMarketID}
		err = client.SubscribeComments(marketIDs)
		if err != nil {
			t.Fatalf("SubscribeComments failed: %v", err)
		}

		// 等待一段时间以接收消息
		time.Sleep(3 * time.Second)

		client.Stop()
		time.Sleep(1 * time.Second)
	})
}

func TestUnsubscribeComments(t *testing.T) {
	client := NewClient(test.DefaultReconnectDelay)
	config := test.LoadTestConfig()

	if config.TestMarketID == "" {
		t.Skip("Skipping test: POLY_TEST_MARKET_ID not set")
	}

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		err := client.Start()
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		// 等待连接建立
		time.Sleep(2 * time.Second)

		// 先订阅
		marketIDs := []string{config.TestMarketID}
		err = client.SubscribeComments(marketIDs)
		if err != nil {
			t.Fatalf("SubscribeComments failed: %v", err)
		}

		time.Sleep(1 * time.Second)

		// 取消订阅
		err = client.UnsubscribeComments(marketIDs)
		if err != nil {
			t.Fatalf("UnsubscribeComments failed: %v", err)
		}

		client.Stop()
		time.Sleep(1 * time.Second)
	})
}
