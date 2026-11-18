package catalog

import (
	"context"
	"fmt"
	"github.com/example/e-commerce-be/internal/database"
	"github.com/example/e-commerce-be/internal/imagevector"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/example/e-commerce-be/internal/outbox"
	"github.com/example/e-commerce-be/internal/productsearch"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Service struct {
	repo             *shared.Repository
	search           productSearch
	searchMode       string
	imageKitEndpoint string
	imageVectors     *imagevector.Client
}

func New(r *shared.Repository) *Service { return &Service{repo: r} }

func (s *Service) SetImageKitEndpoint(endpoint string) {
	s.imageKitEndpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
}

func (s *Service) SetImageSearch(client *imagevector.Client, endpoint string) {
	s.imageVectors = client
	s.SetImageKitEndpoint(endpoint)
}

func (s *Service) SearchByImage(ctx context.Context, data []byte, contentType string, page, limit int) ([]models.Product, error) {
	if s.imageVectors == nil || page < 1 || limit < 1 || limit > 100 {
		return nil, shared.ErrUnavailable
	}
	started := time.Now()
	vector, model, err := s.imageVectors.Embed(ctx, data, contentType)
	if err != nil {
		return nil, fmt.Errorf("create query image embedding: %w", err)
	}
	k := min(page*limit*5, 500)
	ids, err := s.imageVectors.Search(ctx, vector, model, k)
	if err != nil {
		return nil, fmt.Errorf("search image vectors: %w", err)
	}
	products, err := s.hydrateSearchIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	start := (page - 1) * limit
	if start >= len(products) {
		return []models.Product{}, nil
	}
	end := min(start+limit, len(products))
	slog.Info("catalog image search completed", "total_ms", time.Since(started).Milliseconds(), "candidate_count", len(ids), "result_count", end-start)
	return products[start:end], nil
}

func (s *Service) IndexProductImages(ctx context.Context, product uuid.UUID) error {
	if s.imageVectors == nil {
		return nil
	}
	ready, err := s.imageVectors.AliasExists(ctx)
	if err != nil {
		return err
	}
	if !ready {
		return nil
	}
	return s.IndexProductImagesTo(ctx, product, imagevector.Alias)
}

func (s *Service) IndexProductImagesTo(ctx context.Context, product uuid.UUID, target string) error {
	if s.imageVectors == nil {
		return nil
	}
	_, ok, err := s.SearchDocument(ctx, product)
	if err != nil {
		return err
	}
	if !ok {
		return s.imageVectors.DeleteProductFrom(ctx, target, product)
	}
	var images []models.ProductImage
	if err := s.repo.DB.WithContext(ctx).Where("product_id=? AND deleted_at IS NULL", product).Order("sort_order,id").Find(&images).Error; err != nil {
		return err
	}
	if err := s.imageVectors.DeleteProductFrom(ctx, target, product); err != nil {
		return err
	}
	indexed := 0
	for _, image := range images {
		if !validImageKitProductURL(image.URL, s.imageKitEndpoint, product) {
			slog.Warn("skip product image outside ImageKit product folder", "productId", product, "imageId", image.ID)
			continue
		}
		data, contentType, err := s.downloadImageKitImage(ctx, product, image.URL)
		if err != nil {
			return fmt.Errorf("download product image %s: %w", image.ID, err)
		}
		vector, model, err := s.imageVectors.Embed(ctx, data, contentType)
		if err != nil {
			return fmt.Errorf("embed product image %s: %w", image.ID, err)
		}
		if err := s.imageVectors.Index(ctx, target, product, image.ID, model, vector); err != nil {
			return fmt.Errorf("index product image %s: %w", image.ID, err)
		}
		indexed++
	}
	slog.Info("product image vectors indexed", "productId", product, "vector_count", indexed)
	return nil
}

func (s *Service) downloadImageKitImage(ctx context.Context, product uuid.UUID, rawURL string) ([]byte, string, error) {
	base, _ := url.Parse(s.imageKitEndpoint)
	u, err := url.Parse(rawURL)
	if err != nil || base == nil || u.Scheme != "https" || !strings.EqualFold(u.Host, base.Host) || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, "", shared.ErrInvalid
	}
	prefix := strings.TrimRight(base.Path, "/") + "/products/" + product.String() + "/"
	if !strings.HasPrefix(u.Path, prefix) || strings.Contains(strings.TrimPrefix(u.Path, prefix), "/") || strings.TrimPrefix(u.Path, prefix) == "" {
		return nil, "", shared.ErrInvalid
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > 10<<20 {
		return nil, "", fmt.Errorf("image host returned invalid response")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (10<<20)+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 || len(data) > 10<<20 {
		return nil, "", fmt.Errorf("image size is invalid")
	}
	contentType := http.DetectContentType(data)
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		contentType = "image/webp"
	}
	return data, contentType, nil
}

func (s *Service) AddImage(ctx context.Context, user, seller, product uuid.UUID, rawURL string) (models.ProductImage, error) {
	image := models.ProductImage{}
	if !validImageKitProductURL(rawURL, s.imageKitEndpoint, product) {
		return image, shared.ErrInvalid
	}
	if err := s.repo.Seller(ctx, user, seller); err != nil {
		return image, err
	}
	image.ProductID = product
	image.URL = rawURL
	err := s.repo.Within(ctx, func(tx *shared.Repository) error {
		var owned models.Product
		if err := tx.DB.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND seller_id=? AND deleted_at IS NULL", product, seller).First(&owned).Error; err != nil {
			return err
		}
		var maxSort *int
		if err := tx.DB.WithContext(ctx).Model(&models.ProductImage{}).Where("product_id=? AND deleted_at IS NULL", product).Select("MAX(sort_order)").Scan(&maxSort).Error; err != nil {
			return err
		}
		if maxSort != nil {
			image.SortOrder = *maxSort + 1
		}
		if err := tx.Create(ctx, &image); err != nil {
			return err
		}
		return outbox.Enqueue(tx.DB.WithContext(ctx), "product", product, "product_search_updated", map[string]any{"productId": product, "imageId": image.ID})
	})
	return image, err
}

func validImageKitProductURL(rawURL, endpoint string, product uuid.UUID) bool {
	if endpoint == "" {
		return false
	}
	base, err := url.Parse(endpoint)
	if err != nil {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return false
	}
	if !strings.EqualFold(u.Host, base.Host) || u.Scheme != base.Scheme {
		return false
	}
	basePath := strings.TrimRight(base.Path, "/")
	prefix := fmt.Sprintf("%s/products/%s/", basePath, product.String())
	return strings.HasPrefix(u.Path, prefix) && len(strings.TrimPrefix(u.Path, prefix)) > 0 && !strings.Contains(strings.TrimPrefix(u.Path, prefix), "/")
}

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
