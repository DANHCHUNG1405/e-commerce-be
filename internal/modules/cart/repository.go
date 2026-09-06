package cart

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
func (r *Repository) Items(ctx context.Context, user uuid.UUID) ([]models.CartItem, error) {
	v := []models.CartItem{}
	err := r.db.WithContext(ctx).Where("cart_id IN (SELECT id FROM carts WHERE user_id=?) AND deleted_at IS NULL", user).Order("id").Limit(100).Find(&v).Error
	return v, err
}
func (r *Repository) Put(ctx context.Context, user, variant uuid.UUID, qty int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c := models.Cart{UserID: user}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&c).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id=?", user).First(&c).Error; err != nil {
			return err
		}
		if qty == 0 {
			return tx.Where("cart_id=? AND variant_id=?", c.ID, variant).Delete(&models.CartItem{}).Error
		}
		var v models.ProductVariant
		if err := tx.Where("id=? AND deleted_at IS NULL AND product_id IN (SELECT p.id FROM products p JOIN sellers s ON s.id=p.seller_id WHERE p.status='published' AND p.deleted_at IS NULL AND s.status='approved' AND s.deleted_at IS NULL)", variant).First(&v).Error; err != nil {
			return err
		}
		if v.Stock < qty {
			return shared.ErrConflict
		}
		var count int64
		if err := tx.Model(&models.CartItem{}).Where("cart_id=? AND variant_id<>?", c.ID, variant).Count(&count).Error; err != nil {
			return err
		}
		if count >= 100 {
			return shared.ErrConflict
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "cart_id"}, {Name: "variant_id"}}, DoUpdates: clause.Assignments(map[string]any{"quantity": qty})}).Create(&models.CartItem{CartID: c.ID, VariantID: variant, Quantity: qty}).Error
	})
}
