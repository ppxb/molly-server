package cache_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"molly-server/internal/infrastructure/config"
	"molly-server/pkg/cache"
	"molly-server/pkg/logger"
)

func newMemCache(t *testing.T) cache.Cache {
	t.Helper()
	log, _ := logger.New(config.LogConfig{Level: "error", LogPath: "stdout"})
	c, err := cache.New(config.CacheConfig{Type: "memory"}, log)
	require.NoError(t, err)
	t.Cleanup(c.Stop)
	return c
}

func TestMemoryCache_SetAndGet(t *testing.T) {
	c := newMemCache(t)

	require.NoError(t, c.Set("key1", "hello", 10))

	val, err := c.Get("key1")
	require.NoError(t, err)
	assert.Equal(t, "hello", val)
}

func TestMemoryCache_Expiry(t *testing.T) {
	c := newMemCache(t)

	require.NoError(t, c.Set("short", "value", 1))
	time.Sleep(1100 * time.Millisecond) // 等待过期

	_, err := c.Get("short")
	assert.Error(t, err, "expired key should return error")
}

func TestMemoryCache_Delete(t *testing.T) {
	c := newMemCache(t)

	require.NoError(t, c.Set("k", "v", 60))
	require.NoError(t, c.Delete("k"))

	_, err := c.Get("k")
	assert.Error(t, err)
}

func TestMemoryCache_Clear(t *testing.T) {
	c := newMemCache(t)

	require.NoError(t, c.Set("a", 1, 60))
	require.NoError(t, c.Set("b", 2, 60))
	c.Clear()

	_, err1 := c.Get("a")
	_, err2 := c.Get("b")
	assert.Error(t, err1)
	assert.Error(t, err2)
}

func TestMemoryCache_NotFound(t *testing.T) {
	c := newMemCache(t)
	_, err := c.Get("nonexistent")
	assert.Error(t, err)
}
