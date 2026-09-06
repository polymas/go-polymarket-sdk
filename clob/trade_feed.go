package clob

import (
	"sync"
	"time"

	"github.com/polymas/go-polymarket-sdk/types"
)

const (
	// tradeFeedRetention 是喂入的 trade 事件在本地保留的时长；结算等待最长
	// 30 秒，10 分钟足够覆盖"事件先于 POST 响应到达"的乱序窗口。
	tradeFeedRetention = 10 * time.Minute
	// tradeFeedPruneThreshold 超过该条数时才做一次过期清理，避免每次插入都扫表。
	tradeFeedPruneThreshold = 2048
	// tradeFeedFallbackPollInterval 是 feed 已激活时 HTTP 兜底轮询的间隔。
	// 正常情况下 user channel 会在链上确认后几百毫秒内推送 transaction_hash，
	// HTTP 只在 WebSocket 掉线或漏推时补位。
)

// tradeFeedFallbackPollInterval 是 feed 已激活时 HTTP 兜底轮询的间隔（var 便于测试缩短）。
var tradeFeedFallbackPollInterval = 2 * time.Second

type tradeFeedEntry struct {
	trade    types.ClobTrade
	recorded time.Time
}

// tradeFeed 缓存业务层从 user WebSocket 频道喂入的 trade 事件，让结算等待
// 优先消费推送而不是每 250ms 逐个 trade ID 轮询 GET /data/trades。
//
// 一旦有事件喂入即视为"激活"：之后 waitForTradeIDs 以事件唤醒为主，HTTP
// 轮询退化为 2 秒一次的兜底。从未喂入时行为与旧版完全一致。
type tradeFeed struct {
	mu      sync.Mutex
	entries map[string]tradeFeedEntry
	notify  chan struct{} // 每次 record 关闭并换新，实现广播唤醒
	armed   bool
}

func newTradeFeed() *tradeFeed {
	return &tradeFeed{
		entries: make(map[string]tradeFeedEntry),
		notify:  make(chan struct{}),
	}
}

func (f *tradeFeed) record(trade types.ClobTrade) {
	if trade.ID == "" {
		return
	}
	now := time.Now()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.armed = true
	// 同一 trade 只允许状态前进：已经拿到 transaction_hash 的不被后到的
	// 早期事件（乱序到达的 MATCHED）覆盖。
	if prev, ok := f.entries[trade.ID]; ok && tradeExecutionResolved(prev.trade) && !tradeExecutionResolved(trade) {
		return
	}
	f.entries[trade.ID] = tradeFeedEntry{trade: trade, recorded: now}
	if len(f.entries) > tradeFeedPruneThreshold {
		cutoff := now.Add(-tradeFeedRetention)
		for id, entry := range f.entries {
			if entry.recorded.Before(cutoff) {
				delete(f.entries, id)
			}
		}
	}
	close(f.notify)
	f.notify = make(chan struct{})
}

func (f *tradeFeed) lookup(id string) (types.ClobTrade, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.entries[id]
	return entry.trade, ok
}

// wait 返回一个在下一次 record 时被关闭的 channel。
func (f *tradeFeed) wait() <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.notify
}

func (f *tradeFeed) active() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.armed
}
