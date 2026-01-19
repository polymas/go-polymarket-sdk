package websocket

import (
	"fmt"
	"sync"
	"time"
)

// UpdateSubscription 使用新的资产ID更新订阅
// 这会发送包含新资产列表的订阅消息
func (w *webSocketClient) UpdateSubscription(assetIDs []string) error {
	w.subscribedMutex.Lock()
	w.subscribedIDs = assetIDs
	w.subscribedMutex.Unlock()
	w.connMutex.Lock()
	conn := w.conn
	w.connMutex.Unlock()

	if conn != nil {
		// Send new subscription message
		subMsg := map[string]interface{}{
			"assets_ids": assetIDs,
			"type":       "MARKET",
		}
		if w.auth != nil {
			subMsg["auth"] = w.auth
		}
		// 订阅更新日志已移除
		return conn.WriteJSON(subMsg)
	}
	return nil
}

// SubscribeAssets 订阅额外的资产（动态订阅）
// 根据Polymarket文档：https://docs.polymarket.com/developers/CLOB/websocket/wss-overview
// 这允许在初始连接后订阅更多资产
func (w *webSocketClient) SubscribeAssets(assetIDs []string) error {
	w.connMutex.Lock()
	conn := w.conn
	w.connMutex.Unlock()

	if conn == nil {
		return fmt.Errorf("WebSocket not connected")
	}

	// Add to subscribed list
	w.subscribedMutex.Lock()
	w.subscribedIDs = append(w.subscribedIDs, assetIDs...)
	w.subscribedMutex.Unlock()

	// Send subscribe message
	subMsg := map[string]interface{}{
		"assets_ids": assetIDs,
		"operation":  "subscribe",
	}
	if w.auth != nil {
		subMsg["auth"] = w.auth
	}

	// 动态订阅日志已移除
	return conn.WriteJSON(subMsg)
}

// UnsubscribeAssets 取消订阅资产（动态取消订阅）
func (w *webSocketClient) UnsubscribeAssets(assetIDs []string) error {
	w.connMutex.Lock()
	conn := w.conn
	w.connMutex.Unlock()

	if conn == nil {
		return fmt.Errorf("WebSocket not connected")
	}

	// Remove from subscribed list
	assetMap := make(map[string]bool)
	for _, id := range assetIDs {
		assetMap[id] = true
	}

	w.subscribedMutex.Lock()
	newSubscribed := []string{}
	for _, id := range w.subscribedIDs {
		if !assetMap[id] {
			newSubscribed = append(newSubscribed, id)
		}
	}
	w.subscribedIDs = newSubscribed
	w.subscribedMutex.Unlock()

	// Send unsubscribe message
	unsubMsg := map[string]interface{}{
		"assets_ids": assetIDs,
		"operation":  "unsubscribe",
	}
	if w.auth != nil {
		unsubMsg["auth"] = w.auth
	}

	// 动态取消订阅日志已移除
	return conn.WriteJSON(unsubMsg)
}

// StartUserChannel 启动 USER 频道 WebSocket 连接（需要认证）
func (w *webSocketClient) StartUserChannel() error {
	if w.auth == nil {
		return fmt.Errorf("authentication required for USER channel")
	}

	w.userRunningMutex.Lock()
	if w.userRunning {
		w.userRunningMutex.Unlock()
		return fmt.Errorf("USER channel already running")
	}

	w.userStopChan = make(chan struct{})
	w.userStopOnce = sync.Once{}
	w.userRunning = true
	w.userRunningMutex.Unlock()

	go w.runUserChannel()
	return nil
}

// StopUserChannel 停止 USER 频道
func (w *webSocketClient) StopUserChannel() {
	w.userRunningMutex.Lock()
	if !w.userRunning {
		w.userRunningMutex.Unlock()
		return
	}
	w.userRunning = false
	w.userRunningMutex.Unlock()

	w.userStopOnce.Do(func() {
		close(w.userStopChan)
	})

	w.userConnMutex.Lock()
	if w.userConn != nil {
		w.userConn.Close()
	}
	w.userConnMutex.Unlock()
}

// runUserChannel 运行 USER 频道的主循环
func (w *webSocketClient) runUserChannel() {
	for {
		select {
		case <-w.userStopChan:
			return
		default:
			if err := w.connectAndListenUserChannel(); err != nil {
				time.Sleep(w.reconnectDelay)
			}
		}
	}
}
