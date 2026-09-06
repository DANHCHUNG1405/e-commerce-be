package catalog

import (
	"context"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/example/e-commerce-be/internal/outbox"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AdjustInventory is a repository operation invoked after service authorization.
func adjustInventory(ctx context.Context, r *shared.Repository, seller, variant uuid.UUID, delta int, note string) error {
	return r.Within(ctx, func(tx *shared.Repository) error {
		var v models.ProductVariant
		if err := tx.DB.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND deleted_at IS NULL AND product_id IN (SELECT id FROM products WHERE seller_id=? AND deleted_at IS NULL)", variant, seller).First(&v).Error; err != nil {
			return err
		}
		if int64(v.Stock)+int64(delta) < 0 || int64(v.Stock)+int64(delta) > 2147483647 {
			return shared.ErrConflict
		}
		if err := tx.DB.WithContext(ctx).Model(&v).Update("stock", gorm.Expr("stock + ?", delta)).Error; err != nil {
			return err
		}
		if err := tx.Create(ctx, &models.InventoryMovement{VariantID: variant, Quantity: delta, Type: "adjustment", Note: note}); err != nil {
			return err
		}
		return outbox.Enqueue(tx.DB.WithContext(ctx), "variant", variant, "inventory_changed", map[string]any{"delta": delta})
	})
}
func (s *Service) Adjust(ctx context.Context, u, seller, variant uuid.UUID, delta int, note string) error {
	if delta == 0 || delta < -1000000 || delta > 1000000 || len(note) == 0 || len(note) > 500 {
		return shared.ErrInvalid
	}
	if err := s.repo.Seller(ctx, u, seller); err != nil {
		return err
	}
	return adjustInventory(ctx, s.repo, seller, variant, delta, note)
}
