package chat

import (
	"context"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/example/e-commerce-be/internal/rediskey"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"time"
)

type RedisLimiter struct{ client *redis.Client }

func NewRedisLimiter(c *redis.Client) *RedisLimiter { return &RedisLimiter{c} }
func (l *RedisLimiter) Allow(ctx context.Context, user uuid.UUID, kind string, limit int, d time.Duration) error {
	key := rediskey.Key("chat", "rate", kind, user.String())
	n, err := l.client.Eval(ctx, `local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('PEXPIRE',KEYS[1],ARGV[1]) end; return n`, []string{key}, d.Milliseconds()).Int64()
	if err != nil {
		return shared.ErrUnavailable
	}
	if n > int64(limit) {
		return shared.ErrRateLimited
	}
	return nil
}
