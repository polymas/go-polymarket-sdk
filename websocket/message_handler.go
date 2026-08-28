package websocket

import (
	"bytes"
	"encoding/json"
	"strconv"
	"time"

	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

func (w *webSocketClient) dispatchMarketMessage(message []byte) {
	message = bytes.TrimSpace(message)
	if len(message) == 0 {
		return
	}
	if message[0] == '[' {
		var messages []json.RawMessage
		if err := json.Unmarshal(message, &messages); err != nil {
			return
		}
		for _, item := range messages {
			w.dispatchMarketMessage(item)
		}
		return
	}

	var envelope struct {
		EventType MarketEventType `json:"event_type"`
	}
	if err := json.Unmarshal(message, &envelope); err != nil {
		return
	}

	var event MarketEvent
	switch envelope.EventType {
	case MarketEventBook:
		event = &MarketBookEvent{}
	case MarketEventPriceChange:
		event = &MarketPriceChangeEvent{}
	case MarketEventLastTradePrice:
		event = &MarketLastTradePriceEvent{}
	case MarketEventTickSizeChange:
		event = &MarketTickSizeChangeEvent{}
	case MarketEventBestBidAsk:
		event = &MarketBestBidAskEvent{}
	case MarketEventNewMarket:
		event = &MarketNewMarketEvent{}
	case MarketEventMarketResolved:
		event = &MarketResolvedEvent{}
	default:
		return
	}
	if err := json.Unmarshal(message, event); err != nil {
		return
	}
	w.emitMarketEvent(event)
}

func (w *webSocketClient) emitMarketEvent(event MarketEvent) {
	w.callbackMutex.RLock()
	marketCallback := w.onMarketEvent
	bookCallback := w.onBookUpdate
	w.callbackMutex.RUnlock()

	if marketCallback != nil {
		marketCallback(event)
	}
	book, ok := event.(*MarketBookEvent)
	if !ok || bookCallback == nil {
		return
	}
	snapshot := marketBookSnapshot(book)
	internal.LogDebug(
		"[WebSocket] 收到订单簿更新: asset_id=%s, best_bid=%v, best_ask=%v",
		book.TokenID.String(),
		snapshot.BestBid,
		snapshot.BestAsk,
	)
	bookCallback(book.TokenID.String(), snapshot)
}

func marketBookSnapshot(event *MarketBookEvent) *types.BookSnapshot {
	var bestBid, bestAsk *types.BookSide
	for _, level := range event.Bids {
		price, priceOK := parseMarketDecimal(level.Price)
		size, sizeOK := parseMarketDecimal(level.Size)
		if !priceOK || !sizeOK {
			continue
		}
		if bestBid == nil || price > bestBid.Price {
			bestBid = &types.BookSide{Price: price, Size: size}
		}
	}
	for _, level := range event.Asks {
		price, priceOK := parseMarketDecimal(level.Price)
		size, sizeOK := parseMarketDecimal(level.Size)
		if !priceOK || !sizeOK {
			continue
		}
		if bestAsk == nil || price < bestAsk.Price {
			bestAsk = &types.BookSide{Price: price, Size: size}
		}
	}
	return &types.BookSnapshot{
		BestBid: bestBid,
		BestAsk: bestAsk,
		TS:      parseMarketTimestamp(event.Timestamp),
	}
}

func parseMarketDecimal(value string) (float64, bool) {
	parsed, err := strconv.ParseFloat(value, 64)
	return parsed, err == nil
}

func parseMarketTimestamp(value string) time.Time {
	milliseconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Now()
	}
	return time.UnixMilli(milliseconds)
}
