package websocket

import (
	"sync"
	"testing"
	"time"

	"github.com/polymas/go-polymarket-sdk/test"
	"github.com/polymas/go-polymarket-sdk/types"
)

func TestSportsSetOnSportsUpdate(t *testing.T) {
	client := NewSportsClient(test.DefaultReconnectDelay)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		called := false
		client.SetOnSportsUpdate(func(event *types.SportsEvent) {
			called = true
		})
		// 设置回调不应该出错
		if !called {
			t.Logf("SetOnSportsUpdate succeeded (callback will be called on message)")
		}
	})
}

func TestSportsSetAuth(t *testing.T) {
	client := NewSportsClient(test.DefaultReconnectDelay)

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

func TestSportsStart(t *testing.T) {
	client := NewSportsClient(test.DefaultReconnectDelay)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		// 设置回调
		var receivedUpdate bool
		var mu sync.Mutex
		client.SetOnSportsUpdate(func(event *types.SportsEvent) {
			mu.Lock()
			receivedUpdate = true
			mu.Unlock()
			t.Logf("Received sports update: %+v", event)
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
		hasUpdate := receivedUpdate
		mu.Unlock()

		if !hasUpdate {
			t.Logf("No updates received (may be expected if no sports events)")
		}
	})
}

func TestSportsStop(t *testing.T) {
	client := NewSportsClient(test.DefaultReconnectDelay)

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

func TestSportsIsRunning(t *testing.T) {
	client := NewSportsClient(test.DefaultReconnectDelay)

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
