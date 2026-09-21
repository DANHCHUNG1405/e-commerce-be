package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/example/e-commerce-be/internal/database"
	"github.com/example/e-commerce-be/internal/events"
	internalKafka "github.com/example/e-commerce-be/internal/kafka"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	kafkago "github.com/segmentio/kafka-go"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	writer := internalKafka.NewWriter(brokers, events.Topic)
	defer writer.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := internalKafka.Wait(ctx, brokers); err != nil {
		slog.Error("kafka unavailable", "error", err)
		os.Exit(1)
	}
	slog.Info("outbox relay started", "brokers", brokers)
	for ctx.Err() == nil {
		if err := relayBatch(ctx, db, writer); err != nil {
			slog.Error("outbox relay batch failed", "error", err)
			time.Sleep(time.Second)
		}
	}
}

func relayBatch(ctx context.Context, db *gorm.DB, writer *kafkago.Writer) error {
	var rows []models.OutboxEvent
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Recover claims left behind by a relay crash.
		tx.Model(&models.OutboxEvent{}).Where("status='publishing' AND available_at < ?", time.Now().UTC().Add(-time.Minute)).Updates(map[string]any{"status": "pending", "available_at": time.Now().UTC()})
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("status='pending' AND available_at <= ?", time.Now().UTC()).Order("created_at,id").Limit(50).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Model(&models.OutboxEvent{}).Where("id IN ?", ids(rows)).Updates(map[string]any{"status": "publishing"}).Error
	})
	if err != nil || len(rows) == 0 {
		if err == nil {
			time.Sleep(500 * time.Millisecond)
		}
		return err
	}
	for _, row := range rows {
		var payload json.RawMessage = row.Payload
		body, err := json.Marshal(events.Envelope{EventID: row.ID, EventType: row.EventType, AggregateType: row.AggregateType, AggregateID: row.AggregateID, OccurredAt: row.CreatedAt.UTC(), Version: 1, Payload: payload})
		if err == nil {
			err = writer.WriteMessages(ctx, kafkago.Message{Key: []byte(row.AggregateID.String()), Value: body})
		}
		if err != nil {
			_ = db.WithContext(ctx).Model(&models.OutboxEvent{}).Where("id=?", row.ID).Updates(map[string]any{"status": "pending", "retry_count": gorm.Expr("retry_count + 1"), "available_at": time.Now().UTC().Add(time.Second)}).Error
			return err
		}
		if err := db.WithContext(ctx).Model(&models.OutboxEvent{}).Where("id=?", row.ID).Updates(map[string]any{"status": "published", "processed_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
	}
	return nil
}

func ids(rows []models.OutboxEvent) []uuid.UUID {
	out := make([]uuid.UUID, len(rows))
	for i := range rows {
		out[i] = rows[i].ID
	}
	return out
}
func value(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
