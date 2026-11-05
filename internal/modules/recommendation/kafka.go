package recommendation

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

func BrokerAddresses(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if address := strings.TrimSpace(part); address != "" {
			out = append(out, address)
		}
	}
	return out
}

func PingAnyBroker(ctx context.Context, raw string) error {
	for _, address := range BrokerAddresses(raw) {
		checkCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		conn, err := kafka.DialContext(checkCtx, "tcp", address)
		cancel()
		if err == nil {
			return conn.Close()
		}
	}
	return errors.New("Kafka unavailable")
}

func NewReader(brokers, topic, group string) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{Brokers: BrokerAddresses(brokers), Topic: topic, GroupID: group, MinBytes: 1, MaxBytes: 10 << 20, CommitInterval: 0})
}

func NewWriter(brokers, topic string) *kafka.Writer {
	return &kafka.Writer{Addr: kafka.TCP(BrokerAddresses(brokers)...), Topic: topic, Balancer: &kafka.Hash{}, RequiredAcks: kafka.RequireAll, BatchTimeout: 20 * time.Millisecond, WriteTimeout: 3 * time.Second}
}
