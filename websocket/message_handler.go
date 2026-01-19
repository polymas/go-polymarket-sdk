package websocket

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// handleOrderUpdate handles order status update messages
func (w *webSocketClient) handleOrderUpdate(msg map[string]interface{}) {
	if w.onOrderUpdate == nil {
		return
	}

	// Parse order from message
	orderJSON, err := json.Marshal(msg)
	if err != nil {
		return
	}

	var order types.OpenOrder
	if err := json.Unmarshal(orderJSON, &order); err != nil {
		return
	}

	w.onOrderUpdate(&order)
}

// handleTradeUpdate handles trade update messages
func (w *webSocketClient) handleTradeUpdate(msg map[string]interface{}) {
	if w.onTradeUpdate == nil {
		return
	}

	// Parse trade from message
	tradeJSON, err := json.Marshal(msg)
	if err != nil {
		return
	}

	var trade types.PolygonTrade
	if err := json.Unmarshal(tradeJSON, &trade); err != nil {
		return
	}

	w.onTradeUpdate(&trade)
}

// handleBookUpdate handles order book update messages
func (w *webSocketClient) handleBookUpdate(msg map[string]interface{}) {
	assetID, ok := msg["asset_id"].(string)
	if !ok {
		return
	}

	// Clean asset ID
	assetID = strings.TrimPrefix(assetID, "0x")

	var bestBid, bestAsk *types.BookSide

	// Parse bids - Python code: bb = self._parse_top(bids, max)
	// Python finds the maximum price from all bids
	if bidsRaw, ok := msg["bids"]; ok && bidsRaw != nil {
		var bids []interface{}
		switch v := bidsRaw.(type) {
		case []interface{}:
			bids = v
		default:
			// Skip invalid bid format
		}

		if len(bids) > 0 {
			// Find best bid (highest price) - Python uses max(parsed, key=lambda x: x[0])
			var bestPrice float64 = -1
			var bestSize float64 = 0
			for _, bidRaw := range bids {
				bid, ok := bidRaw.(map[string]interface{})
				if !ok {
					continue
				}
				price := parseFloat64(bid["price"])
				size := parseFloat64(bid["size"])
				if price > bestPrice {
					bestPrice = price
					bestSize = size
				}
			}
			if bestPrice > 0 {
				bestBid = &types.BookSide{Price: bestPrice, Size: bestSize}
			}
		}
	}

	// Parse asks - Python code: ba = self._parse_top(asks, min)
	// Python finds the minimum price from all asks
	if asksRaw, ok := msg["asks"]; ok && asksRaw != nil {
		var asks []interface{}
		switch v := asksRaw.(type) {
		case []interface{}:
			asks = v
		default:
			// Skip invalid ask format
		}

		if len(asks) > 0 {
			// Find best ask (lowest price) - Python uses min(parsed, key=lambda x: x[0])
			var bestPrice float64 = -1
			var bestSize float64 = 0
			for _, askRaw := range asks {
				ask, ok := askRaw.(map[string]interface{})
				if !ok {
					continue
				}
				price := parseFloat64(ask["price"])
				size := parseFloat64(ask["size"])
				if bestPrice < 0 || price < bestPrice {
					bestPrice = price
					bestSize = size
				}
			}
			if bestPrice > 0 {
				bestAsk = &types.BookSide{Price: bestPrice, Size: bestSize}
			}
		}
	}

	snapshot := &types.BookSnapshot{
		BestBid: bestBid,
		BestAsk: bestAsk,
		TS:      time.Now(),
	}

	// Debug日志：记录订单簿更新
	internal.LogDebug("[WebSocket] 收到订单簿更新: asset_id=%s, best_bid=%v, best_ask=%v",
		assetID, bestBid, bestAsk)

	if w.onBookUpdate != nil {
		w.onBookUpdate(assetID, snapshot)
	}
}

// parseFloat64 safely parses a float64 from interface{}
func parseFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		var f float64
		fmt.Sscanf(val, "%f", &f)
		return f
	default:
		return 0
	}
}
