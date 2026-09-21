package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/example/e-commerce-be/internal/events"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct{ db *gorm.DB }

var ErrInvalidEvent = errors.New("invalid notification event")

type Notification struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	UserID    uuid.UUID      `gorm:"type:uuid" json:"userId"`
	EventID   uuid.UUID      `gorm:"type:uuid" json:"eventId"`
	Type      string         `json:"type"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	Data      map[string]any `gorm:"serializer:json" json:"data"`
	ReadAt    *time.Time     `json:"readAt,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
}

func (Notification) TableName() string { return "notification.notifications" }

type UnreadCount struct {
	Count int64 `json:"count"`
}

func New(db *gorm.DB) *Service { return &Service{db: db} }

func (s *Service) Handle(ctx context.Context, e events.Envelope) (Notification, bool, error) {
	var notification Notification
	userID, status, supported, err := recipient(e)
	if err != nil {
		return notification, false, err
	}
	if !supported {
		return notification, false, nil
	}
	var data map[string]any
	if err := json.Unmarshal(e.Payload, &data); err != nil {
		return notification, false, fmt.Errorf("%w: decode payload: %v", ErrInvalidEvent, err)
	}
	title, body := message(e.EventType, status)
	notification = Notification{ID: uuid.New(), UserID: userID, EventID: e.EventID, Type: e.EventType, Title: title, Body: body, Data: data, CreatedAt: time.Now().UTC()}
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "event_id"}}, DoNothing: true}).Create(&notification)
	if result.Error != nil {
		return Notification{}, false, result.Error
	}
	return notification, result.RowsAffected == 1, nil
}

func recipient(e events.Envelope) (uuid.UUID, string, bool, error) {
	if e.EventID == uuid.Nil || e.Version != 1 || strings.TrimSpace(e.AggregateType) == "" || e.AggregateID == uuid.Nil || e.OccurredAt.IsZero() {
		return uuid.Nil, "", false, ErrInvalidEvent
	}
	switch e.EventType {
	case events.OrderCreated:
		var p events.OrderCreatedPayload
		if json.Unmarshal(e.Payload, &p) != nil || p.OrderID == uuid.Nil || p.UserID == uuid.Nil || p.Total < 0 || p.Discount < 0 || p.Currency == "" {
			return uuid.Nil, "", false, ErrInvalidEvent
		}
		return p.UserID, "", true, nil
	case events.PaymentSucceeded:
		var p events.PaymentSucceededPayload
		if json.Unmarshal(e.Payload, &p) != nil || p.OrderID == uuid.Nil || p.UserID == uuid.Nil || p.Amount < 0 || strings.TrimSpace(p.Method) == "" {
			return uuid.Nil, "", false, ErrInvalidEvent
		}
		return p.UserID, "", true, nil
	case events.OrderStatusChanged:
		var p events.OrderStatusChangedPayload
		if json.Unmarshal(e.Payload, &p) != nil || p.OrderID == uuid.Nil || p.UserID == uuid.Nil || strings.TrimSpace(p.Status) == "" {
			return uuid.Nil, "", false, ErrInvalidEvent
		}
		return p.UserID, p.Status, true, nil
	default:
		return uuid.Nil, "", false, nil
	}
}

func (s *Service) List(ctx context.Context, user uuid.UUID, page, limit int) ([]Notification, error) {
	items := []Notification{}
	err := s.db.WithContext(ctx).Table("notification.notifications").Where("user_id=?", user).Order("created_at DESC,id DESC").Offset((page - 1) * limit).Limit(limit).Find(&items).Error
	return items, err
}

func (s *Service) Unread(ctx context.Context, user uuid.UUID) (UnreadCount, error) {
	var result UnreadCount
	err := s.db.WithContext(ctx).Table("notification.notifications").Where("user_id=? AND read_at IS NULL", user).Count(&result.Count).Error
	return result, err
}

func (s *Service) MarkRead(ctx context.Context, user, id uuid.UUID) error {
	result := s.db.WithContext(ctx).Table("notification.notifications").Where("id=? AND user_id=?", id, user).Updates(map[string]any{"read_at": gorm.Expr("COALESCE(read_at, NOW())")})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Service) MarkAllRead(ctx context.Context, user uuid.UUID) error {
	return s.db.WithContext(ctx).Table("notification.notifications").Where("user_id=? AND read_at IS NULL", user).Update("read_at", gorm.Expr("NOW()")).Error
}

func message(kind, status string) (string, string) {
	switch kind {
	case events.OrderCreated:
		return "New order", "Your order was created successfully."
	case events.PaymentSucceeded:
		return "Payment succeeded", "Your payment was received successfully."
	case events.OrderStatusChanged:
		return "Order updated", "Your order status changed to: " + status
	default:
		return "Account update", "You have a new update."
	}
}
