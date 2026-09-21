package events

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const Topic = "ecommerce.domain-events.v1"

const (
	OrderCreated       = "order_created"
	PaymentSucceeded   = "payment_succeeded"
	OrderStatusChanged = "order_status_changed"
)

type Envelope struct {
	EventID       uuid.UUID       `json:"eventId"`
	EventType     string          `json:"eventType"`
	AggregateType string          `json:"aggregateType"`
	AggregateID   uuid.UUID       `json:"aggregateId"`
	OccurredAt    time.Time       `json:"occurredAt"`
	Version       int             `json:"version"`
	Payload       json.RawMessage `json:"payload"`
}

type OrderCreatedPayload struct {
	OrderID  uuid.UUID `json:"orderId"`
	UserID   uuid.UUID `json:"userId"`
	Total    int64     `json:"total"`
	Discount int64     `json:"discount"`
	Currency string    `json:"currency"`
}

type PaymentSucceededPayload struct {
	OrderID               uuid.UUID `json:"orderId"`
	UserID                uuid.UUID `json:"userId"`
	Amount                int64     `json:"amount"`
	Method                string    `json:"method"`
	ProviderTransactionID string    `json:"providerTransactionId,omitempty"`
}

type OrderStatusChangedPayload struct {
	OrderID       uuid.UUID  `json:"orderId"`
	UserID        uuid.UUID  `json:"userId"`
	Status        string     `json:"status"`
	SellerOrderID *uuid.UUID `json:"sellerOrderId,omitempty"`
}

// RealtimeNotification is the process boundary between Notification Service
// and API instances that own WebSocket connections.
type RealtimeNotification struct {
	RecipientUserID uuid.UUID       `json:"recipientUserId"`
	Notification    json.RawMessage `json:"notification"`
}
