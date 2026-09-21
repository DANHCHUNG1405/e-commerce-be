package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/example/e-commerce-be/internal/events"
	"github.com/google/uuid"
	kafkago "github.com/segmentio/kafka-go"
)

type MessageReader interface {
	FetchMessage(context.Context) (kafkago.Message, error)
	CommitMessages(context.Context, ...kafkago.Message) error
}

type MessageWriter interface {
	WriteMessages(context.Context, ...kafkago.Message) error
}

type EventHandler interface {
	Handle(context.Context, events.Envelope) (Notification, bool, error)
}

type RealtimePublisher interface {
	Publish(context.Context, events.RealtimeNotification) error
}

type Consumer struct {
	reader     MessageReader
	dlq        MessageWriter
	handler    EventHandler
	realtime   RealtimePublisher
	dlqTopic   string
	minBackoff time.Duration
	maxBackoff time.Duration
}

type DeadLetter struct {
	Topic     string    `json:"topic"`
	Partition int       `json:"partition"`
	Offset    int64     `json:"offset"`
	Key       []byte    `json:"key,omitempty"`
	Value     []byte    `json:"value"`
	Error     string    `json:"error"`
	FailedAt  time.Time `json:"failedAt"`
}

func NewConsumer(reader MessageReader, dlq MessageWriter, handler EventHandler, realtime RealtimePublisher, dlqTopic string) *Consumer {
	return &Consumer{reader: reader, dlq: dlq, handler: handler, realtime: realtime, dlqTopic: dlqTopic, minBackoff: time.Second, maxBackoff: 30 * time.Second}
}

func (c *Consumer) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			slog.Error("notification kafka fetch failed", "error", err)
			if !wait(ctx, c.minBackoff) {
				return nil
			}
			continue
		}
		if err := c.process(ctx, msg); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
	return nil
}

func (c *Consumer) process(ctx context.Context, msg kafkago.Message) error {
	var envelope events.Envelope
	if err := json.Unmarshal(msg.Value, &envelope); err != nil {
		return c.deadLetterAndCommit(ctx, msg, fmt.Errorf("decode envelope: %w", err))
	}

	backoff := c.minBackoff
	for {
		notification, created, err := c.handler.Handle(ctx, envelope)
		if err == nil {
			if created && c.realtime != nil {
				body, marshalErr := json.Marshal(notification)
				if marshalErr != nil {
					slog.Error("notification realtime encode failed", "eventId", envelope.EventID, "error", marshalErr)
				} else if publishErr := c.realtime.Publish(ctx, events.RealtimeNotification{RecipientUserID: notification.UserID, Notification: body}); publishErr != nil && ctx.Err() == nil {
					slog.Error("notification realtime publish failed", "eventId", envelope.EventID, "error", publishErr)
				}
			}
			return c.commit(ctx, msg)
		}
		if errors.Is(err, ErrInvalidEvent) {
			return c.deadLetterAndCommit(ctx, msg, err)
		}
		slog.Error("notification handling failed; retrying", "eventId", envelope.EventID, "error", err, "backoff", backoff)
		if !wait(ctx, backoff) {
			return ctx.Err()
		}
		backoff = nextBackoff(backoff, c.maxBackoff)
	}
}

func (c *Consumer) deadLetterAndCommit(ctx context.Context, msg kafkago.Message, cause error) error {
	if c.dlq == nil || c.dlqTopic == "" {
		return errors.New("notification DLQ is not configured")
	}
	body, err := json.Marshal(DeadLetter{Topic: msg.Topic, Partition: msg.Partition, Offset: msg.Offset, Key: msg.Key, Value: msg.Value, Error: cause.Error(), FailedAt: time.Now().UTC()})
	if err != nil {
		return fmt.Errorf("encode notification dead letter: %w", err)
	}
	backoff := c.minBackoff
	for {
		err = c.dlq.WriteMessages(ctx, kafkago.Message{Key: msg.Key, Value: body})
		if err == nil {
			slog.Warn("notification event sent to DLQ", "topic", msg.Topic, "partition", msg.Partition, "offset", msg.Offset, "error", cause)
			return c.commit(ctx, msg)
		}
		slog.Error("notification DLQ publish failed; retrying", "error", err, "backoff", backoff)
		if !wait(ctx, backoff) {
			return ctx.Err()
		}
		backoff = nextBackoff(backoff, c.maxBackoff)
	}
}

func (c *Consumer) commit(ctx context.Context, msg kafkago.Message) error {
	backoff := c.minBackoff
	for {
		if err := c.reader.CommitMessages(ctx, msg); err == nil {
			return nil
		} else if ctx.Err() == nil {
			slog.Error("notification kafka commit failed; retrying", "error", err, "backoff", backoff)
		}
		if !wait(ctx, backoff) {
			return ctx.Err()
		}
		backoff = nextBackoff(backoff, c.maxBackoff)
	}
}

func nextBackoff(current, maximum time.Duration) time.Duration {
	next := current * 2
	if next > maximum {
		return maximum
	}
	return next
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

type RedisPublisher struct {
	publish func(context.Context, string, any) error
	channel string
}

func NewRedisPublisher(channel string, publish func(context.Context, string, any) error) *RedisPublisher {
	return &RedisPublisher{publish: publish, channel: channel}
}

func (p *RedisPublisher) Publish(ctx context.Context, event events.RealtimeNotification) error {
	if p == nil || p.publish == nil || p.channel == "" || event.RecipientUserID == uuid.Nil {
		return errors.New("invalid realtime notification")
	}
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode realtime notification: %w", err)
	}
	return p.publish(ctx, p.channel, body)
}
