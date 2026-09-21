package admin

import (
	"context"

	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Service struct{ db *gorm.DB }

type Dashboard struct {
	Users           int64 `json:"users"`
	PendingSellers  int64 `json:"pendingSellers"`
	Orders          int64 `json:"orders"`
	PendingOrders   int64 `json:"pendingOrders"`
	DeliveredOrders int64 `json:"deliveredOrders"`
	Revenue         int64 `json:"revenue"`
	PendingPayments int64 `json:"pendingPayments"`
}

func New(db *gorm.DB) *Service { return &Service{db: db} }

func (s *Service) Dashboard(ctx context.Context, u uuid.UUID) (Dashboard, error) {
	if err := shared.New(s.db).Admin(ctx, u); err != nil {
		return Dashboard{}, err
	}
	q := s.db.WithContext(ctx)
	var d Dashboard
	if err := q.Model(&models.User{}).Where("deleted_at IS NULL").Count(&d.Users).Error; err != nil {
		return d, err
	}
	if err := q.Model(&models.Seller{}).Where("deleted_at IS NULL AND status='pending'").Count(&d.PendingSellers).Error; err != nil {
		return d, err
	}
	if err := q.Model(&models.Order{}).Where("deleted_at IS NULL").Count(&d.Orders).Error; err != nil {
		return d, err
	}
	if err := q.Model(&models.Order{}).Where("deleted_at IS NULL AND status='pending'").Count(&d.PendingOrders).Error; err != nil {
		return d, err
	}
	if err := q.Model(&models.Order{}).Where("deleted_at IS NULL AND status='delivered'").Count(&d.DeliveredOrders).Error; err != nil {
		return d, err
	}
	if err := q.Model(&models.Order{}).Select("COALESCE(SUM(total),0)").Where("deleted_at IS NULL AND status='delivered'").Scan(&d.Revenue).Error; err != nil {
		return d, err
	}
	if err := q.Model(&models.Payment{}).Where("status IN ('pending','requires_review')").Count(&d.PendingPayments).Error; err != nil {
		return d, err
	}
	return d, nil
}
