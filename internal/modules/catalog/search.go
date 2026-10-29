package catalog

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/example/e-commerce-be/internal/productsearch"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type productSearch interface {
	Search(context.Context, map[string]any, []any, int, int) ([]uuid.UUID, error)
	Suggest(context.Context, string) ([]string, error)
	Index(context.Context, productsearch.Document) error
	Delete(context.Context, uuid.UUID) error
}

func (s *Service) configureSearch(client productSearch, mode string) {
	s.search = client
	s.searchMode = mode
}

func (s *Service) Suggestions(ctx context.Context, q string) ([]string, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []string{}, nil
	}
	if len(q) > 200 {
		return nil, shared.ErrInvalid
	}
	if s.search != nil && s.searchMode != "postgres" {
		if out, err := s.search.Suggest(ctx, q); err == nil {
			return out, nil
		}
	}
	var names []string
	err := s.repo.DB.WithContext(ctx).Model(&models.Product{}).Where("products.deleted_at IS NULL AND products.status='published' AND products.name ILIKE ? AND EXISTS (SELECT 1 FROM seller.sellers ss WHERE ss.id=products.seller_id AND ss.status='approved' AND ss.deleted_at IS NULL)", strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(q)+"%").Order("products.created_at DESC").Limit(5).Pluck("products.name", &names).Error
	return names, err
}

func (s *Service) searchWithES(ctx context.Context, f Filter, page, limit int) ([]models.Product, error) {
	if s.search == nil || (page-1)*limit+limit > 10000 {
		return (&BrowseRepository{db: s.repo.DB}).Search(ctx, f, page, limit)
	}
	from := (page - 1) * limit
	ids, err := s.search.Search(ctx, productsearch.BuildQuery(f.Query, f.SellerID, f.CategoryID, f.MinPrice, f.MaxPrice), productsearch.BuildSort(f.Sort), from, limit)
	if err != nil {
		if s.searchMode == "elasticsearch" {
			slog.Warn("Elasticsearch catalog search failed; using PostgreSQL", "error", err)
		}
		return (&BrowseRepository{db: s.repo.DB}).Search(ctx, f, page, limit)
	}
	products, err := s.hydrateSearchIDs(ctx, ids)
	if err != nil {
		if s.searchMode == "elasticsearch" {
			slog.Warn("Elasticsearch results could not be hydrated; using PostgreSQL", "error", err)
		}
		return (&BrowseRepository{db: s.repo.DB}).Search(ctx, f, page, limit)
	}
	if s.searchMode == "shadow" {
		pg, pgErr := (&BrowseRepository{db: s.repo.DB}).Search(ctx, f, page, limit)
		if pgErr == nil && !sameProductIDs(products, pg) {
			slog.Info("catalog search shadow result mismatch", "page", page, "limit", limit)
		}
		return pg, pgErr
	}
	if len(products) != len(ids) {
		return (&BrowseRepository{db: s.repo.DB}).Search(ctx, f, page, limit)
	}
	return products, nil
}

func (s *Service) hydrateSearchIDs(ctx context.Context, ids []uuid.UUID) ([]models.Product, error) {
	if len(ids) == 0 {
		return []models.Product{}, nil
	}
	var items []models.Product
	if err := s.repo.DB.WithContext(ctx).Where("id IN ? AND deleted_at IS NULL AND status='published'", ids).Find(&items).Error; err != nil {
		return nil, err
	}
	byID := make(map[uuid.UUID]models.Product, len(items))
	sellerIDs := []uuid.UUID{}
	for _, item := range items {
		byID[item.ID] = item
		sellerIDs = append(sellerIDs, item.SellerID)
	}
	shops, err := shared.BatchSellerInfo(ctx, sellerIDs)
	if err != nil {
		return nil, err
	}
	out := make([]models.Product, 0, len(ids))
	for _, id := range ids {
		if item, ok := byID[id]; ok && shops[item.SellerID].Status == "approved" {
			out = append(out, item)
		}
	}
	return out, nil
}
func sameProductIDs(a, b []models.Product) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[uuid.UUID]bool{}
	for _, p := range a {
		set[p.ID] = true
	}
	for _, p := range b {
		if !set[p.ID] {
			return false
		}
	}
	return true
}

// SearchDocument rebuilds an index document from current PostgreSQL state.
func (s *Service) SearchDocument(ctx context.Context, id uuid.UUID) (productsearch.Document, bool, error) {
	doc := productsearch.Document{ID: id, Variants: []productsearch.Variant{}, SKU: []string{}, CategoryIDs: []uuid.UUID{}, Categories: []string{}, Suggest: []string{}}
	var p models.Product
	err := s.repo.DB.WithContext(ctx).Where("id=? AND deleted_at IS NULL", id).First(&p).Error
	if err == gorm.ErrRecordNotFound {
		return doc, false, nil
	}
	if err != nil {
		return doc, false, err
	}
	var seller models.Seller
	if err := s.repo.DB.WithContext(ctx).Where("id=? AND status='approved' AND deleted_at IS NULL", p.SellerID).First(&seller).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return doc, false, nil
		}
		return doc, false, err
	}
	if p.Status != "published" {
		return doc, false, nil
	}
	doc.SellerID = p.SellerID
	doc.SellerName = seller.Name
	doc.Status = p.Status
	doc.Name = p.Name
	doc.Description = p.Description
	doc.CreatedAt = p.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z")
	doc.Suggest = []string{p.Name}
	variants := []models.ProductVariant{}
	if err := s.repo.DB.WithContext(ctx).Model(&models.ProductVariant{}).Where("product_id=? AND deleted_at IS NULL", id).Order("id").Find(&variants).Error; err != nil {
		return doc, false, err
	}
	for _, v := range variants {
		doc.Variants = append(doc.Variants, productsearch.Variant{SKU: v.SKU, Name: v.Name, Price: v.Price})
		doc.SKU = append(doc.SKU, v.SKU)
		if doc.MinPrice == nil || v.Price < *doc.MinPrice {
			price := v.Price
			doc.MinPrice = &price
		}
	}
	if err := s.repo.DB.WithContext(ctx).Table("product_categories pc").Joins("JOIN categories c ON c.id=pc.category_id").Where("pc.product_id=? AND c.deleted_at IS NULL", id).Pluck("pc.category_id", &doc.CategoryIDs).Error; err != nil {
		return doc, false, err
	}
	if err := s.repo.DB.WithContext(ctx).Table("product_categories pc").Joins("JOIN categories c ON c.id=pc.category_id").Where("pc.product_id=? AND c.deleted_at IS NULL", id).Order("pc.category_id").Pluck("c.name", &doc.Categories).Error; err != nil {
		return doc, false, err
	}
	if err := s.repo.DB.WithContext(ctx).Table("order_items i").Joins("JOIN product_variants v ON v.id=i.variant_id").Joins("JOIN seller_orders so ON so.id=i.seller_order_id").Where("v.product_id=? AND so.status='delivered'", id).Select("COALESCE(SUM(i.quantity),0)").Scan(&doc.Sales).Error; err != nil {
		return doc, false, err
	}
	if err := s.repo.DB.WithContext(ctx).Model(&models.Review{}).Where("product_id=? AND status='published' AND deleted_at IS NULL", id).Select("COALESCE(AVG(rating),0)").Scan(&doc.Rating).Error; err != nil {
		return doc, false, err
	}
	return doc, true, nil
}

func (s *Service) IndexProduct(ctx context.Context, id uuid.UUID) error {
	if s.search == nil {
		return fmt.Errorf("Elasticsearch search client is unavailable")
	}
	doc, ok, err := s.SearchDocument(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return s.search.Delete(ctx, id)
	}
	return s.search.Index(ctx, doc)
}
