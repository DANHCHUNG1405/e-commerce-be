package review

import (
	"context"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
)

type Service struct{ repo *shared.Repository }

func New(r *shared.Repository) *Service { return &Service{repo: r} }
func (s *Service) List(ctx context.Context, product uuid.UUID, p, l int) ([]models.Review, error) {
	v := []models.Review{}
	err := s.repo.List(ctx, &v, "product_id=? AND status='published' AND deleted_at IS NULL", []any{product}, p, l)
	return v, err
}

func (s *Service) AdminList(ctx context.Context, u uuid.UUID, status string, product *uuid.UUID, p, l int) ([]models.Review, error) {
	if p < 1 || l < 1 || l > 100 || status != "" && status != "published" && status != "hidden" {
		return nil, shared.ErrInvalid
	}
	if err := s.repo.Admin(ctx, u); err != nil {
		return nil, err
	}
	where := "deleted_at IS NULL"
	args := []any{}
	if status != "" {
		where += " AND status=?"
		args = append(args, status)
	}
	if product != nil {
		where += " AND product_id=?"
		args = append(args, *product)
	}
	items := []models.Review{}
	return items, s.repo.List(ctx, &items, where, args, p, l)
}
func (s *Service) Create(ctx context.Context, u, item uuid.UUID, rating int, comment string) (models.Review, error) {
	v := models.Review{}
	if rating < 1 || rating > 5 || len(comment) > 5000 {
		return v, shared.ErrInvalid
	}
	var line models.OrderItem
	if err := s.repo.One(ctx, &line, "id=? AND seller_order_id IN (SELECT so.id FROM seller_orders so JOIN orders o ON o.id=so.order_id WHERE o.user_id=? AND so.status='delivered')", item, u); err != nil {
		return v, err
	}
	var variant models.ProductVariant
	if err := s.repo.One(ctx, &variant, "id=?", line.VariantID); err != nil {
		return v, err
	}
	v = models.Review{UserID: u, ProductID: variant.ProductID, OrderItemID: &item, Rating: rating, Comment: comment, Status: "published"}
	err := s.repo.Create(ctx, &v)
	return v, err
}
func (s *Service) Hide(ctx context.Context, u, id uuid.UUID) error {
	if err := s.repo.Admin(ctx, u); err != nil {
		return err
	}
	return s.repo.Update(ctx, &models.Review{}, "id=?", []any{id}, map[string]any{"status": "hidden"})
}
