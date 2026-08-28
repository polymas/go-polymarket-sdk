package clob

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/polymas/go-polymarket-sdk/types"
)

type negRiskCacheSource string

const (
	negRiskSourceAPI      negRiskCacheSource = "api"
	negRiskSourceOrderArg negRiskCacheSource = "order_arg"
	negRiskSourceManual   negRiskCacheSource = "manual"
)

type negRiskCacheEntry struct {
	Value     bool
	FetchedAt time.Time
	Source    negRiskCacheSource
	Version   uint64
}

type negRiskFetchCall struct {
	done  chan struct{}
	value bool
	err   error
}

// negRiskCache is deliberately small and non-expiring. A market's negRisk
// classification is immutable, and the official V2 client also caches it for
// the process lifetime. Invalidation exists for operational recovery and tests.
type negRiskCache struct {
	mu       sync.RWMutex
	entries  map[string]negRiskCacheEntry
	inflight map[string]*negRiskFetchCall
	version  uint64
	now      func() time.Time
}

func newNegRiskCache() *negRiskCache {
	return &negRiskCache{
		entries:  make(map[string]negRiskCacheEntry),
		inflight: make(map[string]*negRiskFetchCall),
		now:      time.Now,
	}
}

func canonicalNegRiskTokenID(tokenID string) (string, error) {
	parsed, err := types.ParseTokenID(strings.TrimSpace(tokenID))
	if err != nil || parsed == (types.TokenID{}) {
		if err == nil {
			err = types.ErrInvalidTokenID
		}
		return "", fmt.Errorf("neg risk token: %w", err)
	}
	return parsed.String(), nil
}

func (c *negRiskCache) get(tokenID string) (negRiskCacheEntry, bool) {
	c.mu.RLock()
	entry, ok := c.entries[tokenID]
	c.mu.RUnlock()
	return entry, ok
}

func (c *negRiskCache) set(tokenID string, value bool, source negRiskCacheSource) negRiskCacheEntry {
	c.mu.Lock()
	entry := c.setLocked(tokenID, value, source)
	c.mu.Unlock()
	return entry
}

func (c *negRiskCache) setLocked(tokenID string, value bool, source negRiskCacheSource) negRiskCacheEntry {
	c.version++
	entry := negRiskCacheEntry{
		Value:     value,
		FetchedAt: c.now(),
		Source:    source,
		Version:   c.version,
	}
	c.entries[tokenID] = entry
	return entry
}

func (c *negRiskCache) invalidate(tokenID string) {
	c.mu.Lock()
	delete(c.entries, tokenID)
	c.mu.Unlock()
}

func (c *negRiskCache) getOrFetch(tokenID string, fetch func() (bool, error)) (bool, error) {
	if entry, ok := c.get(tokenID); ok {
		return entry.Value, nil
	}

	c.mu.Lock()
	if entry, ok := c.entries[tokenID]; ok {
		c.mu.Unlock()
		return entry.Value, nil
	}
	if call, ok := c.inflight[tokenID]; ok {
		c.mu.Unlock()
		<-call.done
		return call.value, call.err
	}
	call := &negRiskFetchCall{done: make(chan struct{})}
	c.inflight[tokenID] = call
	c.mu.Unlock()

	call.value, call.err = fetch()

	c.mu.Lock()
	if call.err == nil {
		c.setLocked(tokenID, call.value, negRiskSourceAPI)
	}
	delete(c.inflight, tokenID)
	close(call.done)
	c.mu.Unlock()
	return call.value, call.err
}
