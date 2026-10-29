package notification

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/example/e-commerce-be/internal/events"
	"github.com/google/uuid"
)

func TestRecipientIgnoresValidCatalogEvents(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{})
	e := events.Envelope{EventID: uuid.New(), EventType: events.ProductSearchUpdated, AggregateType: "product", AggregateID: uuid.New(), OccurredAt: time.Now(), Version: 1, Payload: payload}
	_, _, supported, err := recipient(e)
	if err != nil || supported {
		t.Fatalf("expected valid non-notification event to be ignored, supported=%v err=%v", supported, err)
	}
}

func TestRecipientRejectsMalformedCatalogEvents(t *testing.T) {
	e := events.Envelope{EventID: uuid.New(), EventType: events.ReviewSearchUpdated, AggregateType: "product", AggregateID: uuid.New(), OccurredAt: time.Now(), Version: 1, Payload: json.RawMessage(`{}`)}
	_, _, _, err := recipient(e)
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("expected invalid event, got %v", err)
	}
}
