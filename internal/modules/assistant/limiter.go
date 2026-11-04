package assistant

import (
	"context"
	"time"

	"github.com/example/e-commerce-be/internal/rediskey"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type Limiter interface {
	Allow(context.Context, uuid.UUID, int, time.Duration) error
}
type RedisLimiter struct{ client *redis.Client }

func NewRedisLimiter(client *redis.Client) *RedisLimiter { return &RedisLimiter{client: client} }
func (l *RedisLimiter) Allow(ctx context.Context, user uuid.UUID, limit int, window time.Duration) error {
	count, err := l.client.Eval(ctx, `local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('PEXPIRE',KEYS[1],ARGV[1]) end; return n`, []string{rediskey.Key("assistant", "rate", user.String())}, window.Milliseconds()).Int64()
	if err != nil {
		return ErrUnavailable
	}
	if count > int64(limit) {
		return ErrRateLimited
	}
	return nil
}
