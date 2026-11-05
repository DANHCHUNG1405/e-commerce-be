package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/example/e-commerce-be/internal/modules/recommendation"
	"github.com/joho/godotenv"
	"github.com/segmentio/kafka-go"
)

func main() {
	_ = godotenv.Load()
	brokers := os.Getenv("KAFKA_BROKERS")
	addresses := recommendation.BrokerAddresses(brokers)
	if len(addresses) == 0 {
		slog.Error("KAFKA_BROKERS is required")
		os.Exit(1)
	}
	replicas := 1
	if raw := os.Getenv("KAFKA_REPLICATION_FACTOR"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			slog.Error("invalid KAFKA_REPLICATION_FACTOR")
			os.Exit(1)
		}
		replicas = value
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var conn *kafka.Conn
	var err error
	for ctx.Err() == nil {
		for _, address := range addresses {
			tryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			conn, err = kafka.DialContext(tryCtx, "tcp", address)
			cancel()
			if err == nil {
				break
			}
		}
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			break
		case <-time.After(time.Second):
		}
	}
	if err != nil {
		slog.Error("Kafka unavailable", "error", err)
		os.Exit(1)
	}
	defer conn.Close()
	configs := []kafka.TopicConfig{}
	for _, topic := range []string{recommendation.RawTopic, recommendation.EnrichedTopic, recommendation.DeadTopic} {
		configs = append(configs, kafka.TopicConfig{Topic: topic, NumPartitions: 24, ReplicationFactor: replicas, ConfigEntries: []kafka.ConfigEntry{{ConfigName: "retention.ms", ConfigValue: "259200000"}}})
	}
	if err := conn.CreateTopics(configs...); err != nil {
		slog.Error("create behavior topics failed", "error", err)
		os.Exit(1)
	}
	for _, topic := range []string{recommendation.RawTopic, recommendation.EnrichedTopic, recommendation.DeadTopic} {
		partitions, err := conn.ReadPartitions(topic)
		if err != nil || len(partitions) != 24 {
			slog.Error("behavior topic has unexpected partition count", "topic", topic, "partitions", len(partitions), "error", err)
			os.Exit(1)
		}
	}
	slog.Info("behavior topics ready", "partitions", 24, "replicas", replicas)
}
