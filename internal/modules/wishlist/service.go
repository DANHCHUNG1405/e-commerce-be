package wishlist

import (
	"context"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct{ db *gorm.DB }
type Service struct{ repo *Repository }

func New(db *gorm.DB) *Service { return &Service{repo: &Repository{db: db}} }
func (s *Service) List(ctx context.Context, u uuid.UUID, p, l int) ([]models.WishlistItem, error) {
	return s.repo.list(ctx, u, p, l)
}
func (r *Repository) list(ctx context.Context, u uuid.UUID, p, l int) ([]models.WishlistItem, error) {
	v := []models.WishlistItem{}
	err := r.db.WithContext(ctx).Where("wishlist_id IN (SELECT id FROM wishlists WHERE user_id=?)", u).Order("created_at DESC,id").Offset((p - 1) * l).Limit(l).Find(&v).Error
	return v, err
}
func (s *Service) Put(ctx context.Context, u, product uuid.UUID, remove bool) error {
	return s.repo.put(ctx, u, product, remove)
}
func (r *Repository) put(ctx context.Context, u, product uuid.UUID, remove bool) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		w := models.Wishlist{UserID: u}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&w).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id=?", u).First(&w).Error; err != nil {
			return err
		}
		if remove {
			return tx.Where("wishlist_id=? AND product_id=?", w.ID, product).Delete(&models.WishlistItem{}).Error
		}
		var p models.Product
		if err := tx.Where("id=? AND status='published' AND deleted_at IS NULL", product).First(&p).Error; err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.WishlistItem{WishlistID: w.ID, ProductID: product}).Error
	})
}
