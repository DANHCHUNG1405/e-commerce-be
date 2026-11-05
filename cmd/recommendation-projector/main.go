package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/example/e-commerce-be/internal/database"
	"github.com/example/e-commerce-be/internal/modules/recommendation"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	brokers, redisURL := os.Getenv("KAFKA_BROKERS"), os.Getenv("REDIS_URL")
	if len(recommendation.BrokerAddresses(brokers)) == 0 || redisURL == "" {
		slog.Error("recommendation projector configuration missing")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	client, err := database.ConnectRedis(ctx, redisURL)
	if err != nil {
		slog.Error("Redis unavailable", "error", err)
		os.Exit(1)
	}
	defer client.Close()
	reader := recommendation.NewReader(brokers, recommendation.EnrichedTopic, "recommendation-profile-v1")
	defer reader.Close()
	dead := recommendation.NewWriter(brokers, recommendation.DeadTopic)
	defer dead.Close()
	worker := &recommendation.ProfileWorker{Reader: reader, Redis: client, DeadWriter: dead}
	for ctx.Err() == nil {
		if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
			slog.Error("profile worker restarted", "error", err)
			time.Sleep(time.Second)
		}
	}
}
