package voucher

import (
	"context"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) FindCode(ctx context.Context, code string) (Offer, error) {
	var c models.Coupon
	if err := r.db.WithContext(ctx).Where("code=? AND deleted_at IS NULL", code).First(&c).Error; err != nil {
		return Offer{}, err
	}
	return r.Get(ctx, c.ID)
}
func (r *Repository) Used(ctx context.Context, coupon, user uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.CouponRedemption{}).Where("coupon_id=? AND user_id=?", coupon, user).Count(&n).Error
	return n > 0, err
}
func (r *Repository) CartLines(ctx context.Context, user uuid.UUID, ids []uuid.UUID) ([]Line, error) {
	base := r.db.WithContext(ctx).Table("cart_items ci").Joins("JOIN carts c ON c.id=ci.cart_id").Joins("JOIN users u ON u.id=c.user_id").Where("c.user_id=? AND ci.deleted_at IS NULL AND c.deleted_at IS NULL AND u.deleted_at IS NULL", user)
	if len(ids) > 0 {
		base = base.Where("ci.variant_id IN ?", ids)
	}
	var count int64
	if err := base.Count(&count).Error; err != nil {
		return nil, err
	}
	if count < 1 || count > 100 || (len(ids) > 0 && count != int64(len(ids))) {
		return nil, shared.ErrConflict
	}
	v := []Line{}
	err := base.Select("p.seller_id, v.price * ci.quantity AS amount").Joins("JOIN product_variants v ON v.id=ci.variant_id").Joins("JOIN products p ON p.id=v.product_id").Joins("JOIN sellers s ON s.id=p.seller_id").Where("v.deleted_at IS NULL AND p.deleted_at IS NULL AND p.status='published' AND s.deleted_at IS NULL AND s.status='approved' AND ci.quantity BETWEEN 1 AND 10000 AND v.price BETWEEN 0 AND 1000000000000 AND v.stock>=ci.quantity").Order("ci.variant_id").Scan(&v).Error
	if err != nil {
		return nil, err
	}
	if int64(len(v)) != count {
		return nil, shared.ErrConflict
	}
	return v, nil
}
func (r *Repository) Manage(ctx context.Context, user uuid.UUID, seller *uuid.UUID) error {
	if seller == nil {
		return shared.New(r.db).Admin(ctx, user)
	}
	var n int64
	err := r.db.WithContext(ctx).Table("seller_members sm").Joins("JOIN sellers s ON s.id=sm.seller_id").Joins("JOIN users u ON u.id=sm.user_id").Where("sm.user_id=? AND sm.seller_id=? AND sm.role IN ('owner','manager') AND s.status='approved' AND s.deleted_at IS NULL AND u.deleted_at IS NULL", user, *seller).Count(&n).Error
	if err != nil {
		return err
	}
	if n == 0 {
		return shared.ErrForbidden
	}
	return nil
}
func (r *Repository) Create(ctx context.Context, c models.Coupon, rule models.CouponRule) (Offer, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&c).Error; err != nil {
			return err
		}
		rule.CouponID = c.ID
		return tx.Create(&rule).Error
	})
	return Offer{c, []models.CouponRule{rule}}, err
}
func (r *Repository) Get(ctx context.Context, id uuid.UUID) (Offer, error) {
	var o Offer
	if err := r.db.WithContext(ctx).First(&o.Coupon, "id=? AND deleted_at IS NULL", id).Error; err != nil {
		return o, err
	}
	err := r.db.WithContext(ctx).Where("coupon_id=? AND deleted_at IS NULL", id).Find(&o.Rules).Error
	return o, err
}
func (r *Repository) LockCode(ctx context.Context, code string) (Offer, error) {
	var c models.Coupon
	if err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("code=? AND deleted_at IS NULL", code).First(&c).Error; err != nil {
		return Offer{}, err
	}
	return r.Get(ctx, c.ID)
}
func (r *Repository) SetActive(ctx context.Context, id uuid.UUID, active bool) error {
	return r.db.WithContext(ctx).Model(&models.Coupon{}).Where("id=?", id).Update("active", active).Error
}
func (r *Repository) List(ctx context.Context, seller *uuid.UUID, page, limit int) ([]Offer, error) {
	q := r.db.WithContext(ctx).Where("active AND deleted_at IS NULL AND starts_at<=now() AND ends_at>now() AND (usage_limit=0 OR used_count<usage_limit)")
	if seller == nil {
		q = q.Where("id IN (SELECT coupon_id FROM coupon_rules WHERE seller_id IS NULL AND deleted_at IS NULL)")
	} else {
		q = q.Where("id IN (SELECT coupon_id FROM coupon_rules WHERE seller_id=? AND deleted_at IS NULL)", *seller)
	}
	var coupons []models.Coupon
	if err := q.Order("created_at DESC,id").Offset((page - 1) * limit).Limit(limit).Find(&coupons).Error; err != nil {
		return nil, err
	}
	v := []Offer{}
	for _, c := range coupons {
		o, err := r.Get(ctx, c.ID)
		if err != nil {
			return nil, err
		}
		v = append(v, o)
	}
	return v, nil
}
func (r *Repository) Consume(ctx context.Context, coupon, user, order uuid.UUID) error {
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&models.CouponRedemption{CouponID: coupon, UserID: user, OrderID: order})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return shared.ErrConflict
	}
	return r.db.WithContext(ctx).Model(&models.Coupon{}).Where("id=?", coupon).Update("used_count", gorm.Expr("used_count+1")).Error
}
