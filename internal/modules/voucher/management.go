package voucher

import (
	"context"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
)

// Managed lists all lifecycle states, unlike the public available-offers endpoint.
func (s *Service) Managed(ctx context.Context, user uuid.UUID, seller *uuid.UUID, p, l int) ([]Offer, error) {
	if p < 1 || p > 100000 || l < 1 || l > 100 {
		return nil, shared.ErrInvalid
	}
	if err := s.repo.Manage(ctx, user, seller); err != nil {
		return nil, err
	}
	return s.repo.managed(ctx, seller, p, l)
}
func (r *Repository) managed(ctx context.Context, seller *uuid.UUID, p, l int) ([]Offer, error) {
	q := r.db.WithContext(ctx).Where("deleted_at IS NULL")
	if seller == nil {
		q = q.Where("id IN (SELECT coupon_id FROM coupon_rules WHERE seller_id IS NULL AND deleted_at IS NULL)")
	} else {
		q = q.Where("id IN (SELECT coupon_id FROM coupon_rules WHERE seller_id=? AND deleted_at IS NULL)", *seller)
	}
	var rows []models.Coupon
	if err := q.Order("created_at DESC,id").Offset((p - 1) * l).Limit(l).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := []Offer{}
	for _, row := range rows {
		o, err := r.Get(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, o)
	}
	return result, nil
}
