package recommendation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/example/e-commerce-be/internal/rediskey"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

type Collector struct {
	Writer *kafka.Writer
	Redis  *redis.Client
}

var rateScript = redis.NewScript(`local n = redis.call('INCRBY', KEYS[1], ARGV[1]); if n == tonumber(ARGV[1]) then redis.call('EXPIRE', KEYS[1], 60) end; return n`)

func (s *Collector) Collect(ctx context.Context, userID uuid.UUID, inputs []InputEvent) error {
	if len(inputs) == 0 || len(inputs) > 20 {
		return ErrInvalidEvent
	}
	if s.Redis == nil || s.Writer == nil {
		return errors.New("behavior collection unavailable")
	}
	seen := make(map[uuid.UUID]bool, len(inputs))
	messages := make([]kafka.Message, 0, len(inputs))
	now := time.Now().UTC()
	for _, in := range inputs {
		if seen[in.EventID] {
			return ErrInvalidEvent
		}
		seen[in.EventID] = true
		event, err := NewEvent(userID, in, now)
		if err != nil {
			return err
		}
		body, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("marshal behavior event: %w", err)
		}
		messages = append(messages, kafka.Message{Key: []byte(userID.String()), Value: body})
	}
	key := rediskey.Key("behavior", "rate", userID.String(), now.Format("200601021504"))
	n, err := rateScript.Run(ctx, s.Redis, []string{key}, len(messages)).Int64()
	if err != nil {
		return fmt.Errorf("behavior rate limiter: %w", err)
	}
	if n > 120 {
		return ErrRateLimited
	}
	if err := s.Writer.WriteMessages(ctx, messages...); err != nil {
		return fmt.Errorf("publish behavior events: %w", err)
	}
	return nil
}

var ErrRateLimited = errors.New("behavior event rate exceeded")
