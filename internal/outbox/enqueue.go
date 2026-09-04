package outbox

import (
	"encoding/json"
	"time"

	"github.com/example/e-commerce-be/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Enqueue records an event in PostgreSQL as part of the caller's transaction.
// A future consumer can deliver pending events to ClickHouse or other services.
func Enqueue(tx *gorm.DB, aggregateType string, aggregateID uuid.UUID, eventType string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return tx.Create(&models.OutboxEvent{
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		Payload:       body,
		AvailableAt:   time.Now().UTC(),
		Status:        "pending",
	}).Error
}
