package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"molly-server/internal/infrastructure/config"
)

type redisCache struct {
	client *redis.Client
}

func newRedisCache(cfg config.RedisConfig) (*redisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("cache: redis ping failed: %w", err)
	}
	return &redisCache{client: client}, nil
}

func (r *redisCache) Get(key string) (any, error) {
	val, err := r.client.Get(context.Background(), key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, fmt.Errorf("cache: key %q not found", key)
		}
		return nil, fmt.Errorf("cache: redis get %q: %w", key, err)
	}
	return val, nil
}

func (r *redisCache) Set(key string, value any, expireSeconds int) error {
	ttl := time.Duration(expireSeconds) * time.Second
	if expireSeconds <= 0 {
		ttl = 0 // Redis 中 0 表示永不过期
	}
	return r.client.Set(context.Background(), key, value, ttl).Err()
}

func (r *redisCache) Delete(key string) error {
	return r.client.Del(context.Background(), key).Err()
}

func (r *redisCache) Clear() {
	r.client.FlushDB(context.Background())
}

func (r *redisCache) Stop() {
	r.client.Close()
}

var _ Cache = (*redisCache)(nil)
