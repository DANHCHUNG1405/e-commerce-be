package catalog

import (
	"context"
	"github.com/example/e-commerce-be/internal/database"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/example/e-commerce-be/internal/outbox"
	"github.com/example/e-commerce-be/internal/productsearch"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"strings"
)

type Service struct {
	repo       *shared.Repository
	search     productSearch
	searchMode string
}

func New(r *shared.Repository) *Service { return &Service{repo: r} }

func NewWithSearch(r *shared.Repository, es *database.ElasticsearchClient, mode string) *Service {
	s := New(r)
	if es != nil {
		s.configureSearch(productsearch.New(es), mode)
	}
	return s
}

type ProductInput struct {
	Name        string      `json:"name" binding:"required,max=200"`
	Slug        string      `json:"slug" binding:"required,max=200"`
	Description string      `json:"description" binding:"max=10000"`
	Status      string      `json:"status" binding:"required,oneof=draft published archived"`
	CategoryIDs []uuid.UUID `json:"categoryIds"`
}
type VariantInput struct {
	SKU        string         `json:"sku" binding:"required,max=100"`
	Name       string         `json:"name" binding:"required,max=200"`
	Price      int64          `json:"price" binding:"gte=0,lte=1000000000000"`
	Attributes map[string]any `json:"attributes"`
}
type Detail struct {
	Product     models.Product          `json:"product"`
	Variants    []models.ProductVariant `json:"variants"`
	Images      []models.ProductImage   `json:"images"`
	CategoryIDs []uuid.UUID             `json:"categoryIds"`
}

func (s *Service) List(ctx context.Context, q string, p, l int) ([]models.Product, error) {
	return s.Search(ctx, Filter{Query: q}, p, l)
}
func (s *Service) Detail(ctx context.Context, id uuid.UUID) (Detail, error) {
	d := Detail{Variants: []models.ProductVariant{}, Images: []models.ProductImage{}, CategoryIDs: []uuid.UUID{}}
	err := s.repo.One(ctx, &d.Product, "id=? AND deleted_at IS NULL AND status='published'", id)
	if err != nil {
		return d, err
	}
	shop, err := shared.GetSellerInfo(ctx, d.Product.SellerID)
	if err != nil {
		return d, err
	}
	if shop.Status != "approved" {
		return d, gorm.ErrRecordNotFound
	}
	if err = s.repo.List(ctx, &d.Variants, "product_id=? AND deleted_at IS NULL", []any{id}, 1, 100); err != nil {
		return d, err
	}
	err = s.repo.List(ctx, &d.Images, "product_id=? AND deleted_at IS NULL", []any{id}, 1, 100)
	if err == nil {
		err = s.repo.DB.WithContext(ctx).Table("product_categories").Where("product_id=?", id).Order("category_id").Pluck("category_id", &d.CategoryIDs).Error
	}
	return d, err
}
func (s *Service) Save(ctx context.Context, user, seller, id uuid.UUID, in ProductInput) (models.Product, error) {
	v := models.Product{}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Slug) == "" || (in.Status != "draft" && in.Status != "published" && in.Status != "archived") {
		return v, shared.ErrInvalid
	}
	if in.CategoryIDs != nil {
		if len(in.CategoryIDs) > 3 || (in.Status == "published" && len(in.CategoryIDs) == 0) {
			return v, shared.ErrInvalid
		}
		seen := make(map[uuid.UUID]bool, len(in.CategoryIDs))
		for _, categoryID := range in.CategoryIDs {
			if categoryID == uuid.Nil || seen[categoryID] {
				return v, shared.ErrInvalid
			}
			seen[categoryID] = true
		}
	}
	if id == uuid.Nil && in.Status == "published" && len(in.CategoryIDs) == 0 {
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
	wasPublished := v.Status == "published"
	v.SellerID = seller
	v.Name = in.Name
	v.Slug = in.Slug
	v.Description = in.Description
	v.Status = in.Status
	if id == uuid.Nil {
		err := s.repo.Within(ctx, func(tx *shared.Repository) error {
			if err := tx.Create(ctx, &v); err != nil {
				return err
			}
			if err := replaceProductCategories(ctx, tx.DB, v.ID, in.CategoryIDs); err != nil {
				return err
			}
			return outbox.Enqueue(tx.DB.WithContext(ctx), "product", v.ID, "product_search_updated", map[string]any{})
		})
		return v, err
	}
	err := s.repo.Within(ctx, func(tx *shared.Repository) error {
		if in.CategoryIDs == nil && in.Status == "published" && !wasPublished {
			var count int64
			if err := tx.DB.WithContext(ctx).Table("product_categories").Where("product_id=?", id).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return shared.ErrInvalid
			}
		}
		if err := tx.Update(ctx, &models.Product{}, "id=?", []any{id}, map[string]any{"name": v.Name, "slug": v.Slug, "description": v.Description, "status": v.Status}); err != nil {
			return err
		}
		if in.CategoryIDs != nil {
			if err := replaceProductCategories(ctx, tx.DB, id, in.CategoryIDs); err != nil {
				return err
			}
		}
		return outbox.Enqueue(tx.DB.WithContext(ctx), "product", id, "product_search_updated", map[string]any{})
	})
	return v, err
}

func replaceProductCategories(ctx context.Context, db *gorm.DB, productID uuid.UUID, categoryIDs []uuid.UUID) error {
	if categoryIDs == nil {
		return nil
	}
	var count int64
	if len(categoryIDs) > 0 {
		if err := db.WithContext(ctx).Table("categories").Where("id IN ? AND deleted_at IS NULL", categoryIDs).Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(categoryIDs)) {
			return shared.ErrInvalid
		}
	}
	if err := db.WithContext(ctx).Where("product_id=?", productID).Delete(&models.ProductCategory{}).Error; err != nil {
		return err
	}
	for _, categoryID := range categoryIDs {
		if err := db.WithContext(ctx).Create(&models.ProductCategory{ProductID: productID, CategoryID: categoryID}).Error; err != nil {
			return err
		}
	}
	return nil
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
	err := s.repo.Within(ctx, func(tx *shared.Repository) error {
		if err := tx.Create(ctx, &v); err != nil {
			return err
		}
		return outbox.Enqueue(tx.DB.WithContext(ctx), "product", product, "product_search_updated", map[string]any{})
	})
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
