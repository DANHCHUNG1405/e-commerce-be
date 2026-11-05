package catalog

import (
	"context"
	"os"
	"testing"

	"github.com/example/e-commerce-be/internal/database"
	"github.com/google/uuid"
)

// Run with TEST_POSTGRES_URL pointing to a disposable PostgreSQL instance.
func TestReplaceProductCategoriesPostgres(t *testing.T) {
	url := os.Getenv("TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("TEST_POSTGRES_URL is not set")
	}
	db, err := database.Connect(url)
	if err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	for _, statement := range []string{
		`CREATE TEMP TABLE categories (id uuid PRIMARY KEY, deleted_at timestamptz)`,
		`CREATE TEMP TABLE product_categories (product_id uuid NOT NULL, category_id uuid NOT NULL, PRIMARY KEY (product_id,category_id))`,
	} {
		if err := tx.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	product, first, second := uuid.New(), uuid.New(), uuid.New()
	if err := tx.Exec("INSERT INTO categories(id) VALUES (?), (?)", first, second).Error; err != nil {
		t.Fatal(err)
	}
	if err := replaceProductCategories(context.Background(), tx, product, []uuid.UUID{first, second}); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := tx.Table("product_categories").Where("product_id=?", product).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("wanted 2 categories, got %d: %v", count, err)
	}
	if err := replaceProductCategories(context.Background(), tx, product, []uuid.UUID{first, uuid.New()}); err == nil {
		t.Fatal("missing category accepted")
	}
	if err := tx.Table("product_categories").Where("product_id=?", product).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("failed replacement changed rows: %d %v", count, err)
	}
	if err := replaceProductCategories(context.Background(), tx, product, []uuid.UUID{second}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Table("product_categories").Where("product_id=?", product).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("wanted 1 category, got %d: %v", count, err)
	}
}
