package recommendation

import (
	"context"
	"os"
	"testing"

	"github.com/example/e-commerce-be/internal/database"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
)

type testSellers struct{ approved map[uuid.UUID]bool }

func (s testSellers) CheckSellerMembership(context.Context, uuid.UUID, uuid.UUID) (string, string, error) {
	return "", "", nil
}
func (s testSellers) GetSellerInfo(_ context.Context, id uuid.UUID) (shared.SellerInfo, error) {
	if s.approved[id] {
		return shared.SellerInfo{Status: "approved"}, nil
	}
	return shared.SellerInfo{Status: "pending"}, nil
}
func (s testSellers) BatchSellerInfo(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]shared.SellerInfo, error) {
	result := map[uuid.UUID]shared.SellerInfo{}
	for _, id := range ids {
		if s.approved[id] {
			result[id] = shared.SellerInfo{Status: "approved"}
		} else {
			result[id] = shared.SellerInfo{Status: "pending"}
		}
	}
	return result, nil
}
func (s testSellers) GetSellerOperations(context.Context, uuid.UUID) (shared.SellerOperations, error) {
	return shared.SellerOperations{}, nil
}
func (s testSellers) CountSellers(context.Context, string) (int64, error) { return 0, nil }

func TestHydrateRechecksCurrentBusinessState(t *testing.T) {
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
		`CREATE TEMP TABLE products (id uuid PRIMARY KEY, seller_id uuid NOT NULL, name text, slug text, status text, deleted_at timestamptz)`,
		`CREATE TEMP TABLE product_variants (id uuid PRIMARY KEY, product_id uuid NOT NULL, price bigint, stock int, deleted_at timestamptz)`,
		`CREATE TEMP TABLE product_images (id uuid PRIMARY KEY, product_id uuid NOT NULL, url text, sort_order int, deleted_at timestamptz)`,
	} {
		if err := tx.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	approvedSeller, pendingSeller := uuid.New(), uuid.New()
	visible, hidden, outOfStock, pending := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, row := range []struct {
		id, seller uuid.UUID
		status     string
	}{{visible, approvedSeller, "published"}, {hidden, approvedSeller, "draft"}, {outOfStock, approvedSeller, "published"}, {pending, pendingSeller, "published"}} {
		if err := tx.Exec("INSERT INTO products(id,seller_id,name,slug,status) VALUES (?,?,?,?,?)", row.id, row.seller, "Product", "product-"+row.id.String(), row.status).Error; err != nil {
			t.Fatal(err)
		}
		stock := 1
		if row.id == outOfStock {
			stock = 0
		}
		if err := tx.Exec("INSERT INTO product_variants(id,product_id,price,stock) VALUES (?,?,?,?)", uuid.New(), row.id, 1000, stock).Error; err != nil {
			t.Fatal(err)
		}
	}
	ctx := shared.WithSellerDirectory(context.Background(), testSellers{approved: map[uuid.UUID]bool{approvedSeller: true}})
	svc := Service{Repo: NewRepository(tx)}
	cards, err := svc.hydrate(ctx, []uuid.UUID{visible, hidden, outOfStock, pending})
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || cards[0].ID != visible {
		t.Fatalf("wanted only visible approved stocked product; got %+v", cards)
	}
}
