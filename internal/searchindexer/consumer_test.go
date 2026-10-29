package searchindexer

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/example/e-commerce-be/internal/events"
	"github.com/google/uuid"
)

type recordingIndexer struct{ ids []uuid.UUID }

func (r *recordingIndexer) IndexProduct(_ context.Context, id uuid.UUID) error {
	r.ids = append(r.ids, id)
	return nil
}

func TestApplyIgnoresUnrelatedEvent(t *testing.T) {
	indexer := &recordingIndexer{}
	c := &Consumer{indexer: indexer}
	e := events.Envelope{EventID: uuid.New(), EventType: "unrelated_event", AggregateType: "payment", AggregateID: uuid.New(), OccurredAt: time.Now(), Version: 1}
	if err := c.apply(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	if len(indexer.ids) != 0 {
		t.Fatalf("unexpected product indexing: %v", indexer.ids)
	}
}

func TestApplyIndexesProductUpdateIdempotently(t *testing.T) {
	indexer := &recordingIndexer{}
	c := &Consumer{indexer: indexer}
	id := uuid.New()
	e := events.Envelope{EventID: uuid.New(), EventType: events.ProductSearchUpdated, AggregateType: "product", AggregateID: id, OccurredAt: time.Now(), Version: 1, Payload: json.RawMessage(`{}`)}
	if err := c.apply(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	if err := c.apply(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	if len(indexer.ids) != 2 || indexer.ids[0] != id || indexer.ids[1] != id {
		t.Fatalf("expected deterministic rebuild on duplicate, got %v", indexer.ids)
	}
}
