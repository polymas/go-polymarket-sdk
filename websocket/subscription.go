package websocket

import (
	"fmt"
	"sort"
	"strings"
)

// UpdateSubscription replaces the desired token set. When connected it uses
// official subscribe/unsubscribe delta messages; while reconnecting it queues
// the new set for the next initial subscription.
func (w *webSocketClient) UpdateSubscription(tokenIDs []string) error {
	desired := tokenIDSet(tokenIDs)

	w.writeMutex.Lock()
	defer w.writeMutex.Unlock()

	conn := w.currentConn()
	if conn == nil {
		w.replaceSubscribedIDs(desired)
		return nil
	}

	current := w.subscribedIDSet()
	removed := tokenIDSetDifference(current, desired)
	added := tokenIDSetDifference(desired, current)
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

func (w *webSocketClient) SubscribeTokens(tokenIDs []string) error {
	requested := tokenIDSet(tokenIDs)
	if len(requested) == 0 {
		return fmt.Errorf("at least one token ID is required")
	}

	w.writeMutex.Lock()
	defer w.writeMutex.Unlock()
	conn := w.currentConn()
	if conn == nil {
		return fmt.Errorf("Market WebSocket is not connected")
	}

	current := w.subscribedIDSet()
	added := tokenIDSetDifference(requested, current)
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

func (w *webSocketClient) UnsubscribeTokens(tokenIDs []string) error {
	requested := tokenIDSet(tokenIDs)
	if len(requested) == 0 {
		return fmt.Errorf("at least one token ID is required")
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

// SubscribeAssets is kept for source compatibility.
// Deprecated: use SubscribeTokens.
func (w *webSocketClient) SubscribeAssets(tokenIDs []string) error {
	return w.SubscribeTokens(tokenIDs)
}

// UnsubscribeAssets is kept for source compatibility.
// Deprecated: use UnsubscribeTokens.
func (w *webSocketClient) UnsubscribeAssets(tokenIDs []string) error {
	return w.UnsubscribeTokens(tokenIDs)
}

func (w *webSocketClient) writeSubscriptionUpdate(
	conn interface{ WriteJSON(v interface{}) error },
	operation string,
	tokenIDs map[string]struct{},
) error {
	request := marketSubscriptionUpdate{
		Operation:            operation,
		AssetIDs:             tokenIDSetStrings(tokenIDs),
		Level:                w.options.Level,
		CustomFeatureEnabled: w.options.CustomFeatureEnabled,
	}
	if err := conn.WriteJSON(request); err != nil {
		return fmt.Errorf("send Market Channel %s: %w", operation, err)
	}
	return nil
}

func (w *webSocketClient) setSubscribedIDs(tokenIDs []string) {
	w.replaceSubscribedIDs(tokenIDSet(tokenIDs))
}

func (w *webSocketClient) replaceSubscribedIDs(tokenIDs map[string]struct{}) {
	w.subscribedMutex.Lock()
	w.subscribedIDs = tokenIDs
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
	return tokenIDSetStrings(w.subscribedIDSet())
}

func tokenIDSet(tokenIDs []string) map[string]struct{} {
	result := make(map[string]struct{}, len(tokenIDs))
	for _, id := range tokenIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			result[id] = struct{}{}
		}
	}
	return result
}

func tokenIDSetDifference(left, right map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{})
	for id := range left {
		if _, exists := right[id]; !exists {
			result[id] = struct{}{}
		}
	}
	return result
}

func tokenIDSetStrings(tokenIDs map[string]struct{}) []string {
	result := make([]string, 0, len(tokenIDs))
	for id := range tokenIDs {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}
