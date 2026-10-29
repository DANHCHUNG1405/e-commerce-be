package searchindexer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/example/e-commerce-be/internal/events"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/catalog"
	"github.com/google/uuid"
	kafka "github.com/segmentio/kafka-go"
	"gorm.io/gorm"
)

type Reader interface {
	FetchMessage(context.Context) (kafka.Message, error)
	CommitMessages(context.Context, ...kafka.Message) error
}
type Indexer interface {
	IndexProduct(context.Context, uuid.UUID) error
}
type Consumer struct {
	db      *gorm.DB
	reader  Reader
	indexer Indexer
}

func New(db *gorm.DB, reader Reader, indexer Indexer) *Consumer {
	return &Consumer{db: db, reader: reader, indexer: indexer}
}

func (c *Consumer) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			slog.Error("search indexer Kafka fetch failed", "error", err)
			if !pause(ctx, time.Second) {
				return nil
			}
			continue
		}
		if err := c.process(ctx, msg); err != nil && ctx.Err() == nil {
			return err
		}
	}
	return nil
}
func (c *Consumer) process(ctx context.Context, msg kafka.Message) error {
	var event events.Envelope
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		return fmt.Errorf("decode domain event: %w", err)
	}
	for attempt := time.Second; ; attempt = min(attempt*2, 30*time.Second) {
		err := c.apply(ctx, event)
		if err == nil {
			if commitErr := c.reader.CommitMessages(ctx, msg); commitErr == nil {
				return nil
			} else {
				err = commitErr
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		slog.Error("search event processing failed; retrying", "eventId", event.EventID, "error", err, "backoff", attempt)
		if !pause(ctx, attempt) {
			return ctx.Err()
		}
	}
}
func (c *Consumer) apply(ctx context.Context, e events.Envelope) error {
	if e.Version != 1 || e.EventID == uuid.Nil || e.AggregateID == uuid.Nil || e.OccurredAt.IsZero() {
		return errors.New("invalid domain event envelope")
	}
	switch e.EventType {
	case events.ProductSearchUpdated:
		var payload map[string]any
		if e.AggregateType != "product" || json.Unmarshal(e.Payload, &payload) != nil || payload == nil {
			return errors.New("invalid product search aggregate")
		}
		return c.indexer.IndexProduct(ctx, e.AggregateID)
	case events.InventoryChanged:
		var payload map[string]any
		if e.AggregateType != "variant" || json.Unmarshal(e.Payload, &payload) != nil || payload == nil {
			return errors.New("invalid inventory aggregate")
		}
		if _, ok := payload["delta"].(float64); !ok {
			return errors.New("invalid inventory payload")
		}
		var productID uuid.UUID
		if err := c.db.WithContext(ctx).Model(&models.ProductVariant{}).Where("id=?", e.AggregateID).Pluck("product_id", &productID).Error; err != nil {
			return err
		}
		if productID == uuid.Nil {
			return nil
		}
		return c.indexer.IndexProduct(ctx, productID)
	case events.ReviewSearchUpdated:
		var p struct {
			ProductID uuid.UUID `json:"productId"`
		}
		if e.AggregateType != "product" || json.Unmarshal(e.Payload, &p) != nil || p.ProductID == uuid.Nil || p.ProductID != e.AggregateID {
			return errors.New("invalid review search payload")
		}
		return c.indexer.IndexProduct(ctx, p.ProductID)
	case events.SellerStatusChanged, events.SellerSearchUpdated:
		var p struct {
			Status string `json:"status"`
		}
		validPayload := json.Unmarshal(e.Payload, &p) == nil && (e.EventType == events.SellerSearchUpdated || p.Status == "approved" || p.Status == "rejected" || p.Status == "suspended")
		if e.AggregateType != "seller" || !validPayload {
			return errors.New("invalid seller aggregate")
		}
		ids := []uuid.UUID{}
		if err := c.db.WithContext(ctx).Model(&models.Product{}).Where("seller_id=?", e.AggregateID).Pluck("id", &ids).Error; err != nil {
			return err
		}
		for _, id := range ids {
			if err := c.indexer.IndexProduct(ctx, id); err != nil {
				return err
			}
		}
		return nil
	case events.OrderStatusChanged:
		var p events.OrderStatusChangedPayload
		if json.Unmarshal(e.Payload, &p) != nil || p.OrderID == uuid.Nil || p.UserID == uuid.Nil {
			return errors.New("invalid order status payload")
		}
		if p.Status != "delivered" || p.SellerOrderID == nil {
			return nil
		}
		ids := []uuid.UUID{}
		if err := c.db.WithContext(ctx).Table("order_items i").Joins("JOIN product_variants v ON v.id=i.variant_id").Where("i.seller_order_id=?", *p.SellerOrderID).Distinct("v.product_id").Pluck("v.product_id", &ids).Error; err != nil {
			return err
		}
		for _, id := range ids {
			if err := c.indexer.IndexProduct(ctx, id); err != nil {
				return err
			}
		}
		return nil
	default:
		return nil
	}
}
func pause(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

var _ Indexer = (*catalog.Service)(nil)
