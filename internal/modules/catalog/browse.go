package catalog

import (
	"context"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"strings"
)

type Filter struct {
	Query      string     `form:"q"`
	SellerID   *uuid.UUID `form:"-"`
	CategoryID *uuid.UUID `form:"-"`
	MinPrice   *int64     `form:"minPrice"`
	MaxPrice   *int64     `form:"maxPrice"`
	Sort       string     `form:"sort"`
}

func (f Filter) Validate() error {
	if len(f.Query) > 200 || (f.SellerID != nil && *f.SellerID == uuid.Nil) || (f.CategoryID != nil && *f.CategoryID == uuid.Nil) {
		return shared.ErrInvalid
	}
	for _, p := range []*int64{f.MinPrice, f.MaxPrice} {
		if p != nil && (*p < 0 || *p > 1000000000000) {
			return shared.ErrInvalid
		}
	}
	if f.MinPrice != nil && f.MaxPrice != nil && *f.MinPrice > *f.MaxPrice {
		return shared.ErrInvalid
	}
	switch f.Sort {
	case "", "newest", "price_asc", "price_desc", "best_selling", "rating":
		return nil
	default:
		return shared.ErrInvalid
	}
}

type BrowseRepository struct{ db *gorm.DB }

func (s *Service) Search(ctx context.Context, f Filter, page, limit int) ([]models.Product, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	if page < 1 || page > 100000 || limit < 1 || limit > 100 {
		return nil, shared.ErrInvalid
	}
	return (&BrowseRepository{db: s.repo.DB}).Search(ctx, f, page, limit)
}
func (r *BrowseRepository) Search(ctx context.Context, f Filter, page, limit int) ([]models.Product, error) {
	q := r.db.WithContext(ctx).Model(&models.Product{}).Where("products.deleted_at IS NULL AND products.status='published' AND products.seller_id IN (SELECT id FROM sellers WHERE status='approved' AND deleted_at IS NULL)")
	if f.Query != "" {
		q = q.Where("products.name ILIKE ?", "%"+strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(f.Query)+"%")
	}
	if f.SellerID != nil {
		q = q.Where("products.seller_id=?", *f.SellerID)
	}
	if f.CategoryID != nil {
		q = q.Where("EXISTS (SELECT 1 FROM product_categories pc JOIN categories c ON c.id=pc.category_id WHERE pc.product_id=products.id AND c.id=? AND c.deleted_at IS NULL)", *f.CategoryID)
	}
	if f.MinPrice != nil || f.MaxPrice != nil {
		sub := r.db.Table("product_variants v").Select("1").Where("v.product_id=products.id AND v.deleted_at IS NULL")
		if f.MinPrice != nil {
			sub = sub.Where("v.price>=?", *f.MinPrice)
		}
		if f.MaxPrice != nil {
			sub = sub.Where("v.price<=?", *f.MaxPrice)
		}
		q = q.Where("EXISTS (?)", sub)
	}
	switch f.Sort {
	case "price_asc":
		q = q.Order("(SELECT MIN(v.price) FROM product_variants v WHERE v.product_id=products.id AND v.deleted_at IS NULL) ASC NULLS LAST")
	case "price_desc":
		q = q.Order("(SELECT MIN(v.price) FROM product_variants v WHERE v.product_id=products.id AND v.deleted_at IS NULL) DESC NULLS LAST")
	case "best_selling":
		q = q.Order("COALESCE((SELECT SUM(i.quantity) FROM order_items i JOIN product_variants v ON v.id=i.variant_id JOIN seller_orders so ON so.id=i.seller_order_id WHERE v.product_id=products.id AND so.status='delivered'),0) DESC")
	case "rating":
		q = q.Order("COALESCE((SELECT AVG(r.rating) FROM reviews r WHERE r.product_id=products.id AND r.status='published' AND r.deleted_at IS NULL),0) DESC")
	}
	v := []models.Product{}
	err := q.Order("products.created_at DESC, products.id").Offset((page - 1) * limit).Limit(limit).Find(&v).Error
	return v, err
}
func (s *Service) Shop(ctx context.Context, id uuid.UUID) (models.Seller, error) {
	v := models.Seller{}
	err := s.repo.One(ctx, &v, "id=? AND status='approved' AND deleted_at IS NULL", id)
	v.PickupAddress = nil // Public shop pages must not expose private pickup contact details.
	return v, err
}
func (s *Service) SellerProducts(ctx context.Context, user, seller uuid.UUID, page, limit int, statuses ...string) ([]models.Product, error) {
	if page < 1 || page > 100000 || limit < 1 || limit > 100 {
		return nil, shared.ErrInvalid
	}
	if err := s.repo.Seller(ctx, user, seller); err != nil {
		return nil, err
	}
	v := []models.Product{}
	where := "seller_id=? AND deleted_at IS NULL"
	args := []any{seller}
	if len(statuses) > 0 && statuses[0] != "" {
		switch statuses[0] {
		case "draft", "published", "archived":
		default:
			return nil, shared.ErrInvalid
		}
		where += " AND status=?"
		args = append(args, statuses[0])
	}
	err := s.repo.List(ctx, &v, where, args, page, limit)
	return v, err
}

func (s *Service) SellerDetail(ctx context.Context, user, seller, id uuid.UUID) (Detail, error) {
	d := Detail{Variants: []models.ProductVariant{}, Images: []models.ProductImage{}}
	if err := s.repo.Seller(ctx, user, seller); err != nil {
		return d, err
	}
	if err := s.repo.One(ctx, &d.Product, "id=? AND seller_id=? AND deleted_at IS NULL", id, seller); err != nil {
		return d, err
	}
	if err := s.repo.List(ctx, &d.Variants, "product_id=? AND deleted_at IS NULL", []any{id}, 1, 100); err != nil {
		return d, err
	}
	err := s.repo.List(ctx, &d.Images, "product_id=? AND deleted_at IS NULL", []any{id}, 1, 100)
	return d, err
}
func (s *Service) UpdateVariant(ctx context.Context, user, seller, product, variant uuid.UUID, in VariantInput) error {
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 200 || strings.TrimSpace(in.SKU) == "" || len(in.SKU) > 100 || in.Price < 0 || in.Price > 1000000000000 {
		return shared.ErrInvalid
	}
	if err := s.repo.Seller(ctx, user, seller); err != nil {
		return err
	}
	p := models.Product{}
	if err := s.repo.One(ctx, &p, "id=? AND seller_id=? AND deleted_at IS NULL", product, seller); err != nil {
		return err
	}
	return (&BrowseRepository{db: s.repo.DB}).UpdateVariant(ctx, product, variant, in)
}
func (r *BrowseRepository) UpdateVariant(ctx context.Context, product, variant uuid.UUID, in VariantInput) error {
	attributes := in.Attributes
	if attributes == nil {
		attributes = map[string]any{}
	}
	v := models.ProductVariant{SKU: in.SKU, Name: in.Name, Price: in.Price, Attributes: attributes}
	// Never update stock here: inventory adjustments require movement history.
	result := r.db.WithContext(ctx).Model(&models.ProductVariant{}).Where("id=? AND product_id=? AND deleted_at IS NULL", variant, product).Select("SKU", "Name", "Price", "Attributes").Updates(&v)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
