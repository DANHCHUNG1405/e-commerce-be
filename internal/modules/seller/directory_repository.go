package seller

import (
	"context"

	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
)

type MembershipView struct {
	SellerID     uuid.UUID `json:"sellerId"`
	SellerName   string    `json:"sellerName"`
	SellerSlug   string    `json:"sellerSlug"`
	SellerStatus string    `json:"sellerStatus"`
	Role         string    `json:"role"`
}

func (r *ManagementRepository) Batch(ctx context.Context, ids []uuid.UUID) ([]models.Seller, error) {
	if len(ids) == 0 || len(ids) > 100 {
		return nil, shared.ErrInvalid
	}
	shops := []models.Seller{}
	err := r.db.WithContext(ctx).Where("id IN ? AND deleted_at IS NULL", ids).Find(&shops).Error
	return shops, err
}

func (r *ManagementRepository) Memberships(ctx context.Context, user uuid.UUID) ([]MembershipView, error) {
	items := []MembershipView{}
	err := r.db.WithContext(ctx).Table("seller.seller_members sm").Select("sm.seller_id, s.name AS seller_name, s.slug AS seller_slug, s.status AS seller_status, sm.role").Joins("JOIN seller.sellers s ON s.id=sm.seller_id").Where("sm.user_id=? AND s.deleted_at IS NULL", user).Order("s.name, sm.role").Scan(&items).Error
	return items, err
}

func (r *ManagementRepository) MemberIDs(ctx context.Context, id uuid.UUID) ([]uuid.UUID, error) {
	ids := []uuid.UUID{}
	err := r.db.WithContext(ctx).Table("seller.seller_members sm").Select("sm.user_id").Joins("JOIN seller.sellers s ON s.id=sm.seller_id").Joins("JOIN identity.users u ON u.id=sm.user_id").Where("sm.seller_id=? AND sm.role IN ('owner','manager','staff') AND s.deleted_at IS NULL AND u.deleted_at IS NULL", id).Order("sm.user_id").Scan(&ids).Error
	return ids, err
}

func (r *ManagementRepository) CountStatus(ctx context.Context, status string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.Seller{}).Where("deleted_at IS NULL AND status=?", status).Count(&count).Error
	return count, err
}

func (r *ManagementRepository) Admin(ctx context.Context, user uuid.UUID) error {
	return shared.New(r.db).Admin(ctx, user)
}
