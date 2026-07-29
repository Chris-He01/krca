package dashboard

import (
	"time"

	"github.com/go-redis/redis/v7"
)

type redisAdapter struct {
	client *redis.Client
}

// NewRedisAdapter wraps a kedis client for dashboard use.
func NewRedisAdapter(c *redis.Client) RedisClient {
	return &redisAdapter{client: c}
}

func (a *redisAdapter) Set(key string, value interface{}, expiration time.Duration) error {
	return a.client.Set(key, value, expiration).Err()
}

func (a *redisAdapter) Get(key string) (string, error) {
	return a.client.Get(key).Result()
}
