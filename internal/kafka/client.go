package kafka

import (
	"context"
	"time"

	"github.com/segmentio/kafka-go"
)

func NewWriter(brokers, topic string) *kafka.Writer {
	return &kafka.Writer{Addr: kafka.TCP(brokers), Topic: topic, Balancer: &kafka.Hash{}, RequiredAcks: kafka.RequireOne, BatchTimeout: 50 * time.Millisecond}
}

func NewReader(brokers, topic, group string) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{Brokers: []string{brokers}, Topic: topic, GroupID: group, MinBytes: 1, MaxBytes: 10 << 20, CommitInterval: 0})
}

func Wait(ctx context.Context, brokers string) error {
	for {
		conn, err := kafka.DialContext(ctx, "tcp", brokers)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func Ping(ctx context.Context, brokers string) error {
	conn, err := kafka.DialContext(ctx, "tcp", brokers)
	if err != nil {
		return err
	}
	return conn.Close()
}
