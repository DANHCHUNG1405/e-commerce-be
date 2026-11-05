package recommendation

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEventValidationAndWeights(t *testing.T) {
	now := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	user, product := uuid.New(), uuid.New()
	for _, tc := range []struct {
		duration int64
		want     float64
	}{{5000, 1}, {29999, 1}, {30000, 2}, {119999, 2}, {120000, 3}, {300000, 3}} {
		e, err := NewEvent(user, InputEvent{EventID: uuid.New(), Type: ProductView, OccurredAt: now, ProductID: product, DurationMS: tc.duration}, now)
		if err != nil || e.Weight() != tc.want {
			t.Fatalf("duration %d: weight=%v err=%v", tc.duration, e.Weight(), err)
		}
	}
	for _, duration := range []int64{0, 4999, 300001} {
		_, err := NewEvent(user, InputEvent{EventID: uuid.New(), Type: ProductView, OccurredAt: now, ProductID: product, DurationMS: duration}, now)
		if err == nil {
			t.Fatalf("duration %d accepted", duration)
		}
	}
	_, err := NewEvent(user, InputEvent{EventID: uuid.New(), Type: ProductClick, OccurredAt: now.Add(-11 * time.Minute), ProductID: product}, now)
	if err == nil {
		t.Fatal("stale event accepted")
	}
}

func TestSearchQueryRemovedAfterEnrichment(t *testing.T) {
	now := time.Now().UTC()
	e, err := NewEvent(uuid.New(), InputEvent{EventID: uuid.New(), Type: SearchSubmit, OccurredAt: now, Query: "  Giày chạy  "}, now)
	if err != nil {
		t.Fatal(err)
	}
	enriched := e.Enriched([]uuid.UUID{uuid.New()})
	if enriched.Query != "" || len(enriched.QueryHash) != 64 || enriched.Weight() != 1 || len(enriched.CategoryIDs) != 1 {
		t.Fatalf("unexpected enriched event: %+v", enriched)
	}
}

func TestRankPrefersSameCategoryAndReviewConfidence(t *testing.T) {
	category := uuid.New()
	interests := []Interest{{CategoryID: category, Score: 8}}
	match := candidate{Categories: []uuid.UUID{category}, Rating: 4.5, ReviewCount: 50}
	other := candidate{Rating: 4.5, ReviewCount: 50}
	if rank(match, interests, false) <= rank(other, interests, false) {
		t.Fatal("category interest ignored")
	}
	lowConfidence := candidate{Rating: 5, ReviewCount: 1}
	highConfidence := candidate{Rating: 4.8, ReviewCount: 100}
	if rank(highConfidence, nil, false) <= rank(lowConfidence, nil, false) {
		t.Fatal("review count confidence ignored")
	}
}
