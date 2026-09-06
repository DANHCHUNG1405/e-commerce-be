package cart

import (
	"context"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
)

type Service struct{ repo *Repository }

func New(r *Repository) *Service { return &Service{repo: r} }
func (s *Service) Items(ctx context.Context, u uuid.UUID) ([]models.CartItem, error) {
	return s.repo.Items(ctx, u)
}
func (s *Service) Put(ctx context.Context, u, v uuid.UUID, q int) error {
	if u == uuid.Nil || v == uuid.Nil || q < 0 || q > 10000 {
		return shared.ErrInvalid
	}
	return s.repo.Put(ctx, u, v, q)
}
