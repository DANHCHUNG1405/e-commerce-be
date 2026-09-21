package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/example/e-commerce-be/internal/database"
	"github.com/example/e-commerce-be/internal/events"
	"github.com/example/e-commerce-be/internal/kafka"
	"github.com/example/e-commerce-be/internal/modules/notification"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	databaseURL := os.Getenv("DATABASE_URL")
	brokers := value("KAFKA_BROKERS", "kafka:9092")
	if databaseURL == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	db, err := database.Connect(databaseURL)
	if err != nil {
		slog.Error("notification database connection failed", "error", err)
		os.Exit(1)
	}
	if err := database.MigrateNotifications(db); err != nil {
		slog.Error("notification migration failed", "error", err)
		os.Exit(1)
	}
	reader := kafka.NewReader(brokers, events.Topic, "notification-service")
	defer reader.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := kafka.Wait(ctx, brokers); err != nil {
		slog.Error("kafka unavailable", "error", err)
		os.Exit(1)
	}
	service := notification.New(db)
	slog.Info("notification consumer started", "brokers", brokers)
	for ctx.Err() == nil {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() == nil {
				slog.Error("kafka fetch failed", "error", err)
			}
			continue
		}
		var event events.Envelope
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			slog.Error("invalid event", "error", err)
			continue
		}
		if err := service.Handle(ctx, event); err != nil {
			slog.Error("notification handling failed", "eventId", event.EventID, "error", err)
			continue
		}
		if err := reader.CommitMessages(ctx, msg); err != nil {
			slog.Error("kafka commit failed", "error", err)
		}
	}
}

func value(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
