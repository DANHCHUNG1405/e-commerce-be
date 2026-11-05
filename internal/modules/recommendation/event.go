package recommendation

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	RawTopic      = "ecommerce.behavior.raw.v1"
	EnrichedTopic = "ecommerce.behavior.enriched.v1"
	DeadTopic     = "ecommerce.behavior.dead.v1"
	ProductClick  = "product_click"
	ProductView   = "product_view"
	SearchSubmit  = "search_submitted"
	Impression    = "recommendation_impression"
)

var ErrInvalidEvent = errors.New("invalid behavior event")

type InputEvent struct {
	EventID          uuid.UUID   `json:"eventId"`
	Type             string      `json:"type"`
	OccurredAt       time.Time   `json:"occurredAt"`
	ProductID        uuid.UUID   `json:"productId,omitempty"`
	ProductIDs       []uuid.UUID `json:"productIds,omitempty"`
	DurationMS       int64       `json:"durationMs,omitempty"`
	Query            string      `json:"query,omitempty"`
	Source           string      `json:"source,omitempty"`
	RecommendationID uuid.UUID   `json:"recommendationId,omitempty"`
}

type Event struct {
	InputEvent
	Version     int         `json:"version"`
	UserID      uuid.UUID   `json:"userId"`
	ReceivedAt  time.Time   `json:"receivedAt"`
	CategoryIDs []uuid.UUID `json:"categoryIds,omitempty"`
	QueryHash   string      `json:"queryHash,omitempty"`
}

func NewEvent(userID uuid.UUID, input InputEvent, now time.Time) (Event, error) {
	if userID == uuid.Nil || input.EventID == uuid.Nil || input.OccurredAt.IsZero() ||
		input.OccurredAt.Before(now.Add(-10*time.Minute)) || input.OccurredAt.After(now.Add(2*time.Minute)) ||
		len(input.Source) > 30 || !utf8.ValidString(input.Source) {
		return Event{}, ErrInvalidEvent
	}
	input.Query = strings.TrimSpace(input.Query)
	switch input.Type {
	case ProductClick:
		if input.ProductID == uuid.Nil || input.DurationMS != 0 || input.Query != "" || len(input.ProductIDs) != 0 {
			return Event{}, ErrInvalidEvent
		}
	case ProductView:
		if input.ProductID == uuid.Nil || input.DurationMS < 5000 || input.DurationMS > 300000 || input.Query != "" || len(input.ProductIDs) != 0 {
			return Event{}, ErrInvalidEvent
		}
	case SearchSubmit:
		if input.ProductID != uuid.Nil || len(input.ProductIDs) != 0 || input.DurationMS != 0 || input.Query == "" || utf8.RuneCountInString(input.Query) > 200 || !utf8.ValidString(input.Query) {
			return Event{}, ErrInvalidEvent
		}
	case Impression:
		if input.ProductID != uuid.Nil || input.DurationMS != 0 || input.Query != "" || input.RecommendationID == uuid.Nil || len(input.ProductIDs) < 1 || len(input.ProductIDs) > 20 {
			return Event{}, ErrInvalidEvent
		}
		seen := make(map[uuid.UUID]bool, len(input.ProductIDs))
		for _, id := range input.ProductIDs {
			if id == uuid.Nil || seen[id] {
				return Event{}, ErrInvalidEvent
			}
			seen[id] = true
		}
	default:
		return Event{}, ErrInvalidEvent
	}
	return Event{InputEvent: input, Version: 1, UserID: userID, ReceivedAt: now.UTC()}, nil
}

func (e Event) Enriched(categories []uuid.UUID) Event {
	e.CategoryIDs = categories
	if e.Query != "" {
		sum := sha256.Sum256([]byte(strings.ToLower(e.Query)))
		e.QueryHash = hex.EncodeToString(sum[:])
		e.Query = ""
	}
	return e
}

func (e Event) Weight() float64 {
	switch e.Type {
	case SearchSubmit:
		return 1
	case ProductClick:
		return 2
	case ProductView:
		if e.DurationMS >= 120000 {
			return 3
		}
		if e.DurationMS >= 30000 {
			return 2
		}
		return 1
	}
	return 0
}
