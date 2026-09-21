package events

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const Topic = "ecommerce.domain-events.v1"

type Envelope struct {
	EventID       uuid.UUID       `json:"eventId"`
	EventType     string          `json:"eventType"`
	AggregateType string          `json:"aggregateType"`
	AggregateID   uuid.UUID       `json:"aggregateId"`
	OccurredAt    time.Time       `json:"occurredAt"`
	Version       int             `json:"version"`
	Payload       json.RawMessage `json:"payload"`
}
