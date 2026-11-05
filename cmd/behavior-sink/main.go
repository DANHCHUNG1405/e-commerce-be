package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/example/e-commerce-be/internal/modules/recommendation"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	brokers, endpoint := os.Getenv("KAFKA_BROKERS"), os.Getenv("CLICKHOUSE_URL")
	if len(recommendation.BrokerAddresses(brokers)) == 0 || endpoint == "" {
		slog.Error("behavior sink configuration missing")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	store := &recommendation.ClickHouse{URL: endpoint, User: os.Getenv("CLICKHOUSE_USER"), Password: os.Getenv("CLICKHOUSE_PASSWORD")}
	for ctx.Err() == nil {
		if err := store.Migrate(ctx); err == nil {
			break
		} else {
			slog.Warn("ClickHouse migration retry", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}
	reader := recommendation.NewReader(brokers, recommendation.EnrichedTopic, "behavior-clickhouse-v1")
	defer reader.Close()
	dead := recommendation.NewWriter(brokers, recommendation.DeadTopic)
	defer dead.Close()
	worker := &recommendation.ClickHouseSink{Reader: reader, Store: store, DeadWriter: dead}
	for ctx.Err() == nil {
		if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
			slog.Error("ClickHouse sink restarted", "error", err)
			time.Sleep(time.Second)
		}
	}
}
