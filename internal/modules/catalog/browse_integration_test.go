package catalog

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/example/e-commerce-be/internal/database"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
)

// Run database-backed packages serially: go test -p 1 ./... .
func TestBrowseIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable PostgreSQL container")
	}
	db, err := database.Connect(dsn)
	if err != nil {
		t.Fatal("test database connection failed")
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := database.Migrate(db); err != nil {
		t.Fatal("test migration failed")
	}
	create := func(v any) {
		t.Helper()
		if err := db.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	u := models.User{Email: uuid.NewString() + "@example.test", Password: "unused"}
	create(&u)
	shop := models.Seller{Name: "Test shop", Slug: uuid.NewString(), Status: "approved"}
	create(&shop)
	create(&models.SellerMember{SellerID: shop.ID, UserID: u.ID, Role: "owner"})
	category := models.Category{Name: "Test category", Slug: uuid.NewString()}
	create(&category)
	var products []models.Product
	var variants []models.ProductVariant
	for i, status := range []string{"published", "published", "draft"} {
		p := models.Product{SellerID: shop.ID, Name: "Test product", Slug: uuid.NewString(), Status: status}
		create(&p)
		v := models.ProductVariant{ProductID: p.ID, SKU: uuid.NewString(), Name: "Variant", Price: int64((i + 1) * 100), Stock: 2, Attributes: map[string]any{}}
		create(&v)
		create(&models.ProductCategory{ProductID: p.ID, CategoryID: category.ID})
		products = append(products, p)
		variants = append(variants, v)
	}
	s := New(shared.New(db))
	ctx := context.Background()
	for _, sort := range []string{"newest", "price_asc", "price_desc", "best_selling", "rating"} {
		list, err := s.Search(ctx, Filter{SellerID: &shop.ID, CategoryID: &category.ID, Sort: sort}, 1, 20)
		if err != nil || len(list) != 2 {
			t.Fatal("search failed", sort, err)
		}
		if sort == "price_asc" && list[0].ID != products[0].ID {
			t.Fatal("wrong ascending price order")
		}
		if sort == "price_desc" && list[0].ID != products[1].ID {
			t.Fatal("wrong descending price order")
		}
	}
	min, max := int64(150), int64(250)
	list, err := s.Search(ctx, Filter{SellerID: &shop.ID, MinPrice: &min, MaxPrice: &max}, 1, 20)
	if err != nil || len(list) != 1 || list[0].ID != products[1].ID {
		t.Fatal("price filtering failed", err)
	}
	in := VariantInput{SKU: variants[0].SKU, Name: "Updated", Price: 0, Attributes: map[string]any{"color": "red"}}
	if err := s.UpdateVariant(ctx, uuid.New(), shop.ID, products[0].ID, variants[0].ID, in); !errors.Is(err, shared.ErrForbidden) {
		t.Fatal("unauthorized variant update")
	}
	if err := s.UpdateVariant(ctx, u.ID, shop.ID, products[0].ID, variants[0].ID, in); err != nil {
		t.Fatal(err)
	}
	var updated models.ProductVariant
	if err := db.First(&updated, "id=?", variants[0].ID).Error; err != nil || updated.Price != 0 || updated.Stock != 2 || updated.Attributes["color"] != "red" {
		t.Fatal("variant update lost fields or stock", err)
	}
	list, err = s.SellerProducts(ctx, u.ID, shop.ID, 1, 20)
	if err != nil || len(list) != 3 {
		t.Fatal("seller cannot see drafts", err)
	}
}
