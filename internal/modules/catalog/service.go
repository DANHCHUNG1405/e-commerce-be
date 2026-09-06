package catalog

import (
	"context"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
	"strings"
)

type Service struct{ repo *shared.Repository }

func New(r *shared.Repository) *Service { return &Service{repo: r} }

type ProductInput struct {
	Name        string `json:"name" binding:"required,max=200"`
	Slug        string `json:"slug" binding:"required,max=200"`
	Description string `json:"description" binding:"max=10000"`
	Status      string `json:"status" binding:"required,oneof=draft published archived"`
}
type VariantInput struct {
	SKU        string         `json:"sku" binding:"required,max=100"`
	Name       string         `json:"name" binding:"required,max=200"`
	Price      int64          `json:"price" binding:"gte=0,lte=1000000000000"`
	Attributes map[string]any `json:"attributes"`
}
type Detail struct {
	Product  models.Product          `json:"product"`
	Variants []models.ProductVariant `json:"variants"`
	Images   []models.ProductImage   `json:"images"`
}

func (s *Service) List(ctx context.Context, q string, p, l int) ([]models.Product, error) {
	out := []models.Product{}
	err := s.repo.List(ctx, &out, "deleted_at IS NULL AND status='published' AND seller_id IN (SELECT id FROM sellers WHERE status='approved' AND deleted_at IS NULL) AND name ILIKE ?", []any{"%" + q + "%"}, p, l)
	return out, err
}
func (s *Service) Detail(ctx context.Context, id uuid.UUID) (Detail, error) {
	d := Detail{Variants: []models.ProductVariant{}, Images: []models.ProductImage{}}
	err := s.repo.One(ctx, &d.Product, "id=? AND deleted_at IS NULL AND status='published' AND seller_id IN (SELECT id FROM sellers WHERE status='approved' AND deleted_at IS NULL)", id)
	if err != nil {
		return d, err
	}
	if err = s.repo.List(ctx, &d.Variants, "product_id=? AND deleted_at IS NULL", []any{id}, 1, 100); err != nil {
		return d, err
	}
	err = s.repo.List(ctx, &d.Images, "product_id=? AND deleted_at IS NULL", []any{id}, 1, 100)
	return d, err
}
func (s *Service) Save(ctx context.Context, user, seller, id uuid.UUID, in ProductInput) (models.Product, error) {
	v := models.Product{}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Slug) == "" || (in.Status != "draft" && in.Status != "published" && in.Status != "archived") {
		return v, shared.ErrInvalid
	}
	if err := s.repo.Seller(ctx, user, seller); err != nil {
		return v, err
	}
	if id != uuid.Nil {
		if err := s.repo.One(ctx, &v, "id=? AND seller_id=? AND deleted_at IS NULL", id, seller); err != nil {
			return v, err
		}
	}
	v.SellerID = seller
	v.Name = in.Name
	v.Slug = in.Slug
	v.Description = in.Description
	v.Status = in.Status
	if id == uuid.Nil {
		err := s.repo.Create(ctx, &v)
		return v, err
	}
	return v, s.repo.Update(ctx, &models.Product{}, "id=?", []any{id}, map[string]any{"name": v.Name, "slug": v.Slug, "description": v.Description, "status": v.Status})
}
func (s *Service) Variant(ctx context.Context, user, seller, product uuid.UUID, in VariantInput) (models.ProductVariant, error) {
	v := models.ProductVariant{}
	if in.Price < 0 || in.Price > 1000000000000 || strings.TrimSpace(in.SKU) == "" || strings.TrimSpace(in.Name) == "" {
		return v, shared.ErrInvalid
	}
	if err := s.repo.Seller(ctx, user, seller); err != nil {
		return v, err
	}
	var p models.Product
	if err := s.repo.One(ctx, &p, "id=? AND seller_id=? AND deleted_at IS NULL", product, seller); err != nil {
		return v, err
	}
	if in.Attributes == nil {
		in.Attributes = map[string]any{}
	}
	v = models.ProductVariant{ProductID: product, SKU: in.SKU, Name: in.Name, Price: in.Price, Attributes: in.Attributes}
	err := s.repo.Create(ctx, &v)
	return v, err
}
func (s *Service) Categories(ctx context.Context, p, l int) ([]models.Category, error) {
	out := []models.Category{}
	err := s.repo.List(ctx, &out, "deleted_at IS NULL", nil, p, l)
	return out, err
}
func (s *Service) Category(ctx context.Context, user uuid.UUID, name, slug string, parent *uuid.UUID) (models.Category, error) {
	v := models.Category{Name: name, Slug: slug, ParentID: parent}
	if strings.TrimSpace(name) == "" || strings.TrimSpace(slug) == "" {
		return v, shared.ErrInvalid
	}
	if err := s.repo.Admin(ctx, user); err != nil {
		return v, err
	}
	err := s.repo.Create(ctx, &v)
	return v, err
}
