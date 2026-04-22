package cache

import (
	"fmt"

	"molly-server/internal/infrastructure/config"
	"molly-server/pkg/logger"
)

// Cache 缓存接口。
// expire 单位为秒，0 表示永不过期（仅 memory 实现支持）。
type Cache interface {
	Get(key string) (any, error)
	Set(key string, value any, expireSeconds int) error
	Delete(key string) error
	Clear()
	Stop()
}

// New 根据配置创建 Cache 实例。
// 支持 memory（默认）和 redis 两种驱动。
func New(cfg config.CacheConfig, log *logger.Logger) (Cache, error) {
	switch cfg.Type {
	case "redis":
		log.Info("cache: using redis", "addr", cfg.Redis.Addr)
		return newRedisCache(cfg.Redis)
	case "memory", "":
		log.Info("cache: using memory")
		return newMemoryCache(), nil
	default:
		return nil, fmt.Errorf("cache: unsupported type %q (memory | redis)", cfg.Type)
	}
}
