package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/example/e-commerce-be/internal/models"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"gorm.io/gorm"
)

type Worker struct {
	db       *gorm.DB
	events   *mongo.Collection
	interval time.Duration
}

var errNoPendingEvent = errors.New("no pending outbox event")

// Enqueue adds an event to the same GORM transaction as the business write.
func Enqueue(tx *gorm.DB, aggregateType string, aggregateID uuid.UUID, eventType string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return tx.Create(&models.OutboxEvent{AggregateType: aggregateType, AggregateID: aggregateID, EventType: eventType, Payload: body, AvailableAt: time.Now(), Status: "pending"}).Error
}

func NewWorker(db *gorm.DB, client *mongo.Client, databaseName string) *Worker {
	return &Worker{db: db, events: client.Database(databaseName).Collection("domain_events"), interval: time.Second}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processOne(ctx)
		}
	}
}

func (w *Worker) processOne(ctx context.Context) {
	var event models.OutboxEvent
	err := w.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("status = ? AND available_at <= ?", "pending", time.Now()).Order("created_at").Limit(1).Find(&event)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errNoPendingEvent
		}
		return tx.Model(&models.OutboxEvent{}).Where("id = ? AND status = ?", event.ID, "pending").Updates(map[string]any{"status": "processing"}).Error
	})
	if errors.Is(err, errNoPendingEvent) {
		return
	}
	if err != nil {
		slog.Error("outbox event claim failed", "error", err)
		return
	}

	var payload any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		w.fail(event.ID, err)
		return
	}
	doc := bson.M{"_id": event.ID.String(), "aggregate_type": event.AggregateType, "aggregate_id": event.AggregateID.String(), "event_type": event.EventType, "payload": payload, "created_at": event.CreatedAt}
	_, err = w.events.ReplaceOne(ctx, bson.M{"_id": event.ID.String()}, doc, options.Replace().SetUpsert(true))
	if err != nil {
		w.fail(event.ID, err)
		return
	}
	_ = w.db.Model(&models.OutboxEvent{}).Where("id = ?", event.ID).Updates(map[string]any{"status": "processed", "processed_at": time.Now()}).Error
}

func (w *Worker) fail(id uuid.UUID, err error) {
	slog.Error("outbox event delivery failed", "event_id", id, "error", err)
	_ = w.db.Model(&models.OutboxEvent{}).Where("id = ?", id).Updates(map[string]any{"status": "pending", "retry_count": gorm.Expr("retry_count + 1"), "available_at": time.Now().Add(10 * time.Second)}).Error
}
