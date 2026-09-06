package database

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// ConnectRedis creates and verifies a Redis client. A rediss:// URL enables TLS.
func ConnectRedis(ctx context.Context, redisURL string) (*redis.Client, error) {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse Redis URL: %w", err)
	}
	client := redis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping Redis: %w", err)
	}
	return client, nil
}
