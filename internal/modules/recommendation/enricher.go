package recommendation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/example/e-commerce-be/internal/productsearch"
	"github.com/example/e-commerce-be/internal/rediskey"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

type Enricher struct {
	Repo       *Repository
	Redis      *redis.Client
	Search     *productsearch.Client
	Reader     *kafka.Reader
	Writer     *kafka.Writer
	DeadWriter *kafka.Writer
}

// Run publishes batches before committing raw offsets. Replayed batches are deduplicated downstream.
func (w *Enricher) Run(ctx context.Context) error {
	batch := make([]kafka.Message, 0, 200)
	for ctx.Err() == nil {
		waitCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		message, err := w.Reader.FetchMessage(waitCtx)
		cancel()
		if err != nil && ctx.Err() != nil {
			break
		}
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if err == nil {
			batch = append(batch, message)
		}
		if len(batch) >= 200 || (err != nil && len(batch) > 0) {
			if err := w.processBatch(ctx, batch); err != nil {
				return err
			}
			if err := w.Reader.CommitMessages(ctx, batch...); err != nil {
				return err
			}
			batch = batch[:0]
		}
	}
	return ctx.Err()
}

func (w *Enricher) processBatch(ctx context.Context, batch []kafka.Message) error {
	valid := make([]kafka.Message, 0, len(batch))
	dead := make([]kafka.Message, 0)
	lookups := make(map[string][]uuid.UUID)
	for _, message := range batch {
		var event Event
		if err := json.Unmarshal(message.Value, &event); err != nil || event.Version != 1 || event.UserID == uuid.Nil || event.EventID == uuid.Nil || string(message.Key) != event.UserID.String() {
			dead = append(dead, deadMessage(message, "invalid event envelope"))
			continue
		}
		if _, err := NewEvent(event.UserID, event.InputEvent, event.ReceivedAt); err != nil {
			dead = append(dead, deadMessage(message, "invalid event fields"))
			continue
		}
		lookupKey := event.Type + ":" + event.ProductID.String() + ":" + strings.ToLower(event.Query)
		categories, ok := lookups[lookupKey]
		if !ok {
			var err error
			categories, err = w.categories(ctx, event)
			if err != nil {
				return err
			}
			lookups[lookupKey] = categories
		}
		body, err := json.Marshal(event.Enriched(categories))
		if err != nil {
			return err
		}
		valid = append(valid, kafka.Message{Key: message.Key, Value: body})
	}
	if len(valid) > 0 {
		if err := w.Writer.WriteMessages(ctx, valid...); err != nil {
			return err
		}
	}
	if len(dead) > 0 {
		if w.DeadWriter == nil {
			return errors.New("behavior dead letter writer unavailable")
		}
		if err := w.DeadWriter.WriteMessages(ctx, dead...); err != nil {
			return err
		}
	}
	return nil
}

func (w *Enricher) categories(ctx context.Context, event Event) ([]uuid.UUID, error) {
	switch event.Type {
	case ProductClick, ProductView:
		key := rediskey.Key("behavior", "product-categories", event.ProductID.String())
		if cached, err := w.Redis.Get(ctx, key).Bytes(); err == nil {
			var ids []uuid.UUID
			if json.Unmarshal(cached, &ids) == nil {
				return ids, nil
			}
		}
		ids, err := w.Repo.PublishedCategories(ctx, event.ProductID)
		if err != nil {
			return nil, fmt.Errorf("load behavior product categories: %w", err)
		}
		body, _ := json.Marshal(ids)
		_ = w.Redis.Set(ctx, key, body, 5*time.Minute).Err()
		return ids, nil
	case SearchSubmit:
		if w.Search == nil {
			return nil, nil
		}
		sum := sha256.Sum256([]byte(strings.ToLower(event.Query)))
		key := rediskey.Key("behavior", "query-categories", hex.EncodeToString(sum[:]))
		if cached, err := w.Redis.Get(ctx, key).Bytes(); err == nil {
			var categories []uuid.UUID
			if json.Unmarshal(cached, &categories) == nil {
				return categories, nil
			}
		}
		// Query text is kept only in the raw Kafka topic; normalized events retain a hash.
		ids, err := w.Search.Search(ctx, productsearch.BuildQuery(event.Query, nil, nil, nil, nil), productsearch.BuildSort("relevance"), 0, 5)
		if err != nil {
			return nil, nil
		}
		if len(ids) == 0 {
			return nil, nil
		}
		categories, err := w.Repo.SearchCategories(ctx, ids)
		if err != nil {
			return nil, err
		}
		body, _ := json.Marshal(categories)
		_ = w.Redis.Set(ctx, key, body, 5*time.Minute).Err()
		return categories, nil
	case Impression:
		return nil, nil
	default:
		return nil, ErrInvalidEvent
	}
}

func deadMessage(message kafka.Message, reason string) kafka.Message {
	sum := sha256.Sum256(message.Value)
	data, _ := json.Marshal(map[string]any{"reason": reason, "sha256": hex.EncodeToString(sum[:]), "topic": message.Topic, "partition": message.Partition, "offset": message.Offset, "at": time.Now().UTC()})
	return kafka.Message{Key: message.Key, Value: data}
}
