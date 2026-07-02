package redis

import (
	"github.com/redis/go-redis/v9"
)

// New creates a Redis client from a URL (e.g. redis://:password@localhost:6379/0).
func New(redisURL string) (*redis.Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}

	client := redis.NewClient(opts)
	return client, nil
}
