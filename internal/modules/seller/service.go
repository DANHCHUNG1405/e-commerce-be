package seller

import (
	"context"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
	"strings"
)

type Service struct{ repo *shared.Repository }

func New(r *shared.Repository) *Service { return &Service{repo: r} }
func (s *Service) Create(ctx context.Context, user uuid.UUID, name, slug string) (models.Seller, error) {
	v := models.Seller{Name: name, Slug: slug, Status: "pending"}
	if strings.TrimSpace(name) == "" || strings.TrimSpace(slug) == "" {
		return v, shared.ErrInvalid
	}
	err := s.repo.Within(ctx, func(r *shared.Repository) error {
		if err := r.Create(ctx, &v); err != nil {
			return err
		}
		return r.Create(ctx, &models.SellerMember{SellerID: v.ID, UserID: user, Role: "owner"})
	})
	return v, err
}
func (s *Service) Mine(ctx context.Context, user uuid.UUID, p, l int) ([]models.Seller, error) {
	v := []models.Seller{}
	err := s.repo.List(ctx, &v, "deleted_at IS NULL AND id IN (SELECT seller_id FROM seller_members WHERE user_id=?)", []any{user}, p, l)
	return v, err
}
func (s *Service) Status(ctx context.Context, user, id uuid.UUID, status string) error {
	if status != "approved" && status != "rejected" && status != "suspended" {
		return shared.ErrInvalid
	}
	if err := s.repo.Admin(ctx, user); err != nil {
		return err
	}
	return s.repo.Update(ctx, &models.Seller{}, "id=? AND deleted_at IS NULL", []any{id}, map[string]any{"status": status})
}
