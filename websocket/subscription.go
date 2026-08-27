package websocket

import (
	"fmt"
	"sort"
	"strings"
)

// UpdateSubscription replaces the desired asset set. When connected it uses
// official subscribe/unsubscribe delta messages; while reconnecting it queues
// the new set for the next initial subscription.
func (w *webSocketClient) UpdateSubscription(assetIDs []string) error {
	desired := assetIDSet(assetIDs)

	w.writeMutex.Lock()
	defer w.writeMutex.Unlock()

	conn := w.currentConn()
	if conn == nil {
		w.replaceSubscribedIDs(desired)
		return nil
	}

	current := w.subscribedIDSet()
	removed := assetIDSetDifference(current, desired)
	added := assetIDSetDifference(desired, current)
	if len(removed) > 0 {
		if err := w.writeSubscriptionUpdate(conn, "unsubscribe", removed); err != nil {
			_ = conn.Close()
			return err
		}
	}
	if len(added) > 0 {
		if err := w.writeSubscriptionUpdate(conn, "subscribe", added); err != nil {
			_ = conn.Close()
			return err
		}
	}
	w.replaceSubscribedIDs(desired)
	return nil
}

func (w *webSocketClient) SubscribeAssets(assetIDs []string) error {
	requested := assetIDSet(assetIDs)
	if len(requested) == 0 {
		return fmt.Errorf("at least one asset ID is required")
	}

	w.writeMutex.Lock()
	defer w.writeMutex.Unlock()
	conn := w.currentConn()
	if conn == nil {
		return fmt.Errorf("Market WebSocket is not connected")
	}

	current := w.subscribedIDSet()
	added := assetIDSetDifference(requested, current)
	if len(added) == 0 {
		return nil
	}
	if err := w.writeSubscriptionUpdate(conn, "subscribe", added); err != nil {
		return err
	}
	for id := range added {
		current[id] = struct{}{}
	}
	w.replaceSubscribedIDs(current)
	return nil
}

func (w *webSocketClient) UnsubscribeAssets(assetIDs []string) error {
	requested := assetIDSet(assetIDs)
	if len(requested) == 0 {
		return fmt.Errorf("at least one asset ID is required")
	}

	w.writeMutex.Lock()
	defer w.writeMutex.Unlock()
	conn := w.currentConn()
	if conn == nil {
		return fmt.Errorf("Market WebSocket is not connected")
	}

	current := w.subscribedIDSet()
	removed := make(map[string]struct{})
	for id := range requested {
		if _, exists := current[id]; exists {
			removed[id] = struct{}{}
		}
	}
	if len(removed) == 0 {
		return nil
	}
	if err := w.writeSubscriptionUpdate(conn, "unsubscribe", removed); err != nil {
		return err
	}
	for id := range removed {
		delete(current, id)
	}
	w.replaceSubscribedIDs(current)
	return nil
}

func (w *webSocketClient) writeSubscriptionUpdate(
	conn interface{ WriteJSON(v interface{}) error },
	operation string,
	assetIDs map[string]struct{},
) error {
	request := marketSubscriptionUpdate{
		Operation:            operation,
		AssetIDs:             assetIDSetStrings(assetIDs),
		Level:                w.options.Level,
		CustomFeatureEnabled: w.options.CustomFeatureEnabled,
	}
	if err := conn.WriteJSON(request); err != nil {
		return fmt.Errorf("send Market Channel %s: %w", operation, err)
	}
	return nil
}

func (w *webSocketClient) setSubscribedIDs(assetIDs []string) {
	w.replaceSubscribedIDs(assetIDSet(assetIDs))
}

func (w *webSocketClient) replaceSubscribedIDs(assetIDs map[string]struct{}) {
	w.subscribedMutex.Lock()
	w.subscribedIDs = assetIDs
	w.subscribedMutex.Unlock()
}

func (w *webSocketClient) subscribedIDSet() map[string]struct{} {
	w.subscribedMutex.RLock()
	defer w.subscribedMutex.RUnlock()
	result := make(map[string]struct{}, len(w.subscribedIDs))
	for id := range w.subscribedIDs {
		result[id] = struct{}{}
	}
	return result
}

func (w *webSocketClient) subscribedIDStrings() []string {
	return assetIDSetStrings(w.subscribedIDSet())
}

func assetIDSet(assetIDs []string) map[string]struct{} {
	result := make(map[string]struct{}, len(assetIDs))
	for _, id := range assetIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			result[id] = struct{}{}
		}
	}
	return result
}

func assetIDSetDifference(left, right map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{})
	for id := range left {
		if _, exists := right[id]; !exists {
			result[id] = struct{}{}
		}
	}
	return result
}

func assetIDSetStrings(assetIDs map[string]struct{}) []string {
	result := make([]string, 0, len(assetIDs))
	for id := range assetIDs {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}
