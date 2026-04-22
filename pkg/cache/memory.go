package cache

import (
	"fmt"
	"sync"
	"time"
)

type entry struct {
	value     any
	expiresAt time.Time
}

func (e *entry) expired() bool {
	return time.Now().After(e.expiresAt)
}

// memoryCache 线程安全的内存 KV 缓存，支持 TTL 和后台自动清理。
type memoryCache struct {
	mu       sync.RWMutex
	items    map[string]*entry
	stopCh   chan struct{}
	stopOnce sync.Once
}

const defaultCleanupInterval = time.Minute

func newMemoryCache() *memoryCache {
	c := &memoryCache{
		items:  make(map[string]*entry),
		stopCh: make(chan struct{}),
	}
	go c.cleanupLoop(defaultCleanupInterval)
	return c
}

func (c *memoryCache) Get(key string) (any, error) {
	c.mu.RLock()
	e, ok := c.items[key]
	c.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("cache: key %q not found", key)
	}
	// 惰性删除：读取时检查是否已过期
	if e.expired() {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return nil, fmt.Errorf("cache: key %q expired", key)
	}
	return e.value, nil
}

func (c *memoryCache) Set(key string, value any, expireSeconds int) error {
	ttl := time.Duration(expireSeconds) * time.Second
	if expireSeconds <= 0 {
		// 永不过期：设置一个极远的时间
		ttl = 100 * 365 * 24 * time.Hour
	}
	c.mu.Lock()
	c.items[key] = &entry{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
	c.mu.Unlock()
	return nil
}

func (c *memoryCache) Delete(key string) error {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
	return nil
}

func (c *memoryCache) Clear() {
	c.mu.Lock()
	c.items = make(map[string]*entry)
	c.mu.Unlock()
}

// Stop 停止后台清理协程，应在 graceful shutdown 时调用。
func (c *memoryCache) Stop() {
	c.stopOnce.Do(func() { close(c.stopCh) })
}

// cleanupLoop 定期清除过期 key，防止内存无限增长。
func (c *memoryCache) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.deleteExpired()
		case <-c.stopCh:
			return
		}
	}
}

func (c *memoryCache) deleteExpired() {
	now := time.Now()
	c.mu.Lock()
	for k, e := range c.items {
		if now.After(e.expiresAt) {
			delete(c.items, k)
		}
	}
	c.mu.Unlock()
}

// 编译期接口检查
var _ Cache = (*memoryCache)(nil)
