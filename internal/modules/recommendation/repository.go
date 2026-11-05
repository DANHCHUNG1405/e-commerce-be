package recommendation

import (
	"context"

	"github.com/example/e-commerce-be/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repository owns PostgreSQL reads used by the recommendation domain.
type Repository struct{ DB *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{DB: db} }

func (r *Repository) Product(ctx context.Context, id uuid.UUID) (models.Product, error) {
	var product models.Product
	err := r.DB.WithContext(ctx).Where("id=? AND deleted_at IS NULL AND status='published'", id).First(&product).Error
	return product, err
}

func (r *Repository) ProductSource(ctx context.Context, id uuid.UUID) (models.Product, error) {
	var product models.Product
	err := r.DB.WithContext(ctx).Select("name", "description").Where("id=?", id).First(&product).Error
	return product, err
}

func (r *Repository) Categories(ctx context.Context, productID uuid.UUID) ([]uuid.UUID, error) {
	ids := []uuid.UUID{}
	err := r.DB.WithContext(ctx).Table("product_categories").Where("product_id=?", productID).Pluck("category_id", &ids).Error
	return ids, err
}

func (r *Repository) PublishedCategories(ctx context.Context, productID uuid.UUID) ([]uuid.UUID, error) {
	ids := []uuid.UUID{}
	err := r.DB.WithContext(ctx).Table("products p").Joins("JOIN product_categories pc ON pc.product_id=p.id").Joins("JOIN categories c ON c.id=pc.category_id AND c.deleted_at IS NULL").Where("p.id=? AND p.status='published' AND p.deleted_at IS NULL", productID).Pluck("pc.category_id", &ids).Error
	return ids, err
}

func (r *Repository) SearchCategories(ctx context.Context, productIDs []uuid.UUID) ([]uuid.UUID, error) {
	var categories []uuid.UUID
	err := r.DB.WithContext(ctx).Raw("SELECT category_id FROM product_categories WHERE product_id IN ? GROUP BY category_id ORDER BY COUNT(*) DESC LIMIT 3", productIDs).Scan(&categories).Error
	return categories, err
}

func (r *Repository) RecentFallback(ctx context.Context) ([]candidate, error) {
	var products []models.Product
	err := r.DB.WithContext(ctx).Where("status='published' AND deleted_at IS NULL AND EXISTS (SELECT 1 FROM product_variants v WHERE v.product_id=products.id AND v.stock>0 AND v.deleted_at IS NULL)").Order("created_at DESC").Limit(80).Find(&products).Error
	if err != nil {
		return nil, err
	}
	items := make([]candidate, 0, len(products))
	for _, p := range products {
		items = append(items, candidate{ID: p.ID, SellerID: p.SellerID, CreatedAt: p.CreatedAt})
	}
	return items, nil
}

func (r *Repository) Cards(ctx context.Context, ids []uuid.UUID) ([]Card, error) {
	if len(ids) == 0 {
		return []Card{}, nil
	}
	var cards []Card
	err := r.DB.WithContext(ctx).Raw(`SELECT p.id, p.seller_id, p.name, p.slug, v.price AS min_price, COALESCE(img.url,'') AS image_url
		FROM products p
		JOIN LATERAL (SELECT price FROM product_variants WHERE product_id=p.id AND stock>0 AND deleted_at IS NULL ORDER BY price LIMIT 1) v ON true
		LEFT JOIN LATERAL (SELECT url FROM product_images WHERE product_id=p.id AND deleted_at IS NULL ORDER BY sort_order LIMIT 1) img ON true
		WHERE p.id IN ? AND p.status='published' AND p.deleted_at IS NULL`, ids).Scan(&cards).Error
	return cards, err
}
