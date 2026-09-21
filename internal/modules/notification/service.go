package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/example/e-commerce-be/internal/events"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct{ db *gorm.DB }

type Notification struct {
	ID        uuid.UUID      `json:"id"`
	UserID    uuid.UUID      `json:"userId"`
	EventID   uuid.UUID      `json:"eventId"`
	Type      string         `json:"type"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	Data      map[string]any `json:"data"`
	ReadAt    *time.Time     `json:"readAt,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
}

type UnreadCount struct {
	Count int64 `json:"count"`
}

func New(db *gorm.DB) *Service { return &Service{db: db} }

func (s *Service) Handle(ctx context.Context, e events.Envelope) error {
	var p struct {
		UserID  uuid.UUID `json:"userId"`
		Status  string    `json:"status"`
		OrderID uuid.UUID `json:"orderId"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("decode event payload: %w", err)
	}
	if p.UserID == uuid.Nil {
		return nil
	}
	title, body := message(e.EventType, p.Status)
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "event_id"}}, DoNothing: true}).Exec(`INSERT INTO notification.notifications (id,user_id,event_id,type,title,body,data) VALUES (?, ?, ?, ?, ?, ?, ?::jsonb)`, uuid.New(), p.UserID, e.EventID, e.EventType, title, body, e.Payload).Error
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
	case "order_created":
		return "New order", "Your order was created successfully."
	case "payment_succeeded":
		return "Payment succeeded", "Your payment was received successfully."
	case "order_status_changed":
		return "Order updated", "Your order status changed to: " + status
	default:
		return "Account update", "You have a new update."
	}
}
