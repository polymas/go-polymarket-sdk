package cache

import (
	"sync"
	"time"
)

// Cache 缓存接口
type Cache interface {
	// Get 获取缓存值
	Get(key string) (interface{}, bool)

	// Set 设置缓存值
	Set(key string, value interface{}, ttl time.Duration)

	// Delete 删除缓存值
	Delete(key string)

	// Clear 清空所有缓存
	Clear()

	// Size 返回缓存大小
	Size() int
}

// entry 缓存条目
type entry struct {
	value      interface{}
	expiration time.Time
}

// IsExpired 检查是否过期
func (e *entry) IsExpired() bool {
	return !e.expiration.IsZero() && time.Now().After(e.expiration)
}

// memoryCache 内存缓存实现
type memoryCache struct {
	mu    sync.RWMutex
	items map[string]*entry
}

// NewMemoryCache 创建内存缓存
func NewMemoryCache() Cache {
	return &memoryCache{
		items: make(map[string]*entry),
	}
}

// Get 获取缓存值
func (c *memoryCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, ok := c.items[key]
	if !ok {
		return nil, false
	}

	// 检查是否过期
	if item.IsExpired() {
		// 异步删除过期项
		go c.Delete(key)
		return nil, false
	}

	return item.value, true
}

// Set 设置缓存值
func (c *memoryCache) Set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var expiration time.Time
	if ttl > 0 {
		expiration = time.Now().Add(ttl)
	}

	c.items[key] = &entry{
		value:      value,
		expiration: expiration,
	}
}

// Delete 删除缓存值
func (c *memoryCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// Clear 清空所有缓存
func (c *memoryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*entry)
}

// Size 返回缓存大小
func (c *memoryCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// noopCache 空操作缓存（禁用缓存时使用）
type noopCache struct{}

// NewNoopCache 创建空操作缓存
func NewNoopCache() Cache {
	return &noopCache{}
}

func (c *noopCache) Get(key string) (interface{}, bool) {
	return nil, false
}

func (c *noopCache) Set(key string, value interface{}, ttl time.Duration) {
	// 不做任何操作
}

func (c *noopCache) Delete(key string) {
	// 不做任何操作
}

func (c *noopCache) Clear() {
	// 不做任何操作
}

func (c *noopCache) Size() int {
	return 0
}
