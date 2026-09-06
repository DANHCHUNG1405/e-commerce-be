package order

import (
	"context"
	"github.com/example/e-commerce-be/internal/database"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/cart"
	"github.com/google/uuid"
	"os"
	"sync"
	"testing"
)

// TEST_DATABASE_URL must point to a disposable local PostgreSQL database.
// Tests create fixtures with random IDs; never load the application's .env.
func TestCheckoutIntegration(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable PostgreSQL database")
	}
	db, err := database.Connect(url)
	if err != nil {
		t.Fatal("test database connection failed")
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err = database.Migrate(db); err != nil {
		t.Fatal("test migration failed", err)
	}
	if err = database.Migrate(db); err != nil {
		t.Fatal("migration rerun failed", err)
	}
	create := func(v any) {
		t.Helper()
		if e := db.Create(v).Error; e != nil {
			t.Fatal(e)
		}
	}
	u := models.User{Email: uuid.NewString() + "@example.test", Password: "unused"}
	create(&u)
	a := models.ShippingAddress{UserID: u.ID, RecipientName: "Original", Country: "VN"}
	create(&a)
	c := models.Cart{UserID: u.ID}
	create(&c)
	variants := []models.ProductVariant{}
	for i := 0; i < 2; i++ {
		seller := models.Seller{Name: "shop", Slug: uuid.NewString(), Status: "approved"}
		create(&seller)
		p := models.Product{SellerID: seller.ID, Name: "Original product", Slug: uuid.NewString(), Status: "published"}
		create(&p)
		v := models.ProductVariant{ProductID: p.ID, SKU: uuid.NewString(), Name: "variant", Price: 100, Stock: 2, Attributes: map[string]any{}}
		create(&v)
		variants = append(variants, v)
		create(&models.CartItem{CartID: c.ID, VariantID: v.ID, Quantity: 1})
	}
	repo := NewRepository(db)
	key := uuid.NewString()
	ctx := context.Background()
	o, err := repo.Checkout(ctx, u.ID, a.ID, key)
	if err != nil {
		t.Fatal(err)
	}
	repeat, err := repo.Checkout(ctx, u.ID, a.ID, key)
	if err != nil || repeat.ID != o.ID {
		t.Fatal("idempotency failed")
	}
	d, err := repo.Detail(ctx, u.ID, o.ID)
	if err != nil || len(d.Sellers) != 2 || len(d.Items) != 2 || d.Order.Total != 200 {
		t.Fatal("marketplace split failed", err)
	}
	if err = db.Model(&a).Update("recipient_name", "Changed").Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&models.ProductVariant{}).Where("id=?", variants[0].ID).Update("price", 200).Error; err != nil {
		t.Fatal(err)
	}
	d, err = repo.Detail(ctx, u.ID, o.ID)
	if err != nil || d.Order.AddressSnapshot["RecipientName"] != "Original" || d.Items[0].UnitPrice != 100 {
		t.Fatal("snapshot changed")
	}
	var events int64
	db.Model(&models.OutboxEvent{}).Where("aggregate_id=?", o.ID).Count(&events)
	if events != 1 {
		t.Fatal("missing or duplicate outbox event")
	}
	if err = repo.Cancel(ctx, u.ID, o.ID); err != nil {
		t.Fatal(err)
	}
	if err = repo.Cancel(ctx, u.ID, o.ID); err != nil {
		t.Fatal(err)
	}
	var check models.ProductVariant
	db.First(&check, "id=?", variants[0].ID)
	if check.Stock != 2 {
		t.Fatal("inventory restoration duplicated")
	}
	// Two customers compete for the same last unit.
	db.Model(&check).Update("stock", 1)
	type buyer struct{ u, a uuid.UUID }
	buyers := []buyer{}
	for i := 0; i < 2; i++ {
		u2 := models.User{Email: uuid.NewString() + "@example.test", Password: "unused"}
		create(&u2)
		a2 := models.ShippingAddress{UserID: u2.ID, Country: "VN"}
		create(&a2)
		if e := cart.NewRepository(db).Put(ctx, u2.ID, check.ID, 1); e != nil {
			t.Fatal(e)
		}
		buyers = append(buyers, buyer{u2.ID, a2.ID})
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, b := range buyers {
		wg.Add(1)
		go func(b buyer) { defer wg.Done(); _, e := repo.Checkout(ctx, b.u, b.a, uuid.NewString()); results <- e }(b)
	}
	wg.Wait()
	close(results)
	wins := 0
	for e := range results {
		if e == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("expected one checkout, got %d", wins)
	}
	db.First(&check, "id=?", check.ID)
	if check.Stock != 0 {
		t.Fatal("invalid remaining inventory")
	}

	t.Run("selected checkout", func(t *testing.T) {
		// The original cart was emptied; restore two different lines.
		if err := db.Model(&models.ProductVariant{}).Where("id IN ?", []uuid.UUID{variants[0].ID, variants[1].ID}).Update("stock", 2).Error; err != nil {
			t.Fatal(err)
		}
		for _, v := range variants {
			if err := cart.NewRepository(db).Put(ctx, u.ID, v.ID, 1); err != nil {
				t.Fatal(err)
			}
		}
		service := New(repo)
		selection := []uuid.UUID{variants[0].ID}
		key := uuid.NewString()
		selected, err := service.CheckoutSelected(ctx, u.ID, a.ID, key, "cod", selection)
		if err != nil {
			t.Fatal(err)
		}
		detail, err := service.Detail(ctx, u.ID, selected.ID)
		if err != nil || len(detail.Items) != 1 {
			t.Fatal("wrong selected lines", err)
		}
		var remaining []models.CartItem
		if err := db.Where("cart_id=?", c.ID).Find(&remaining).Error; err != nil || len(remaining) != 1 || remaining[0].VariantID != variants[1].ID {
			t.Fatal("unselected item removed", err)
		}
		if _, err := service.CheckoutSelected(ctx, u.ID, a.ID, key, "cod", []uuid.UUID{variants[1].ID}); err == nil {
			t.Fatal("changed idempotency payload accepted")
		}
		repeated, err := service.CheckoutSelected(ctx, u.ID, a.ID, key, "cod", selection)
		if err != nil || repeated.ID != selected.ID {
			t.Fatal("selected retry failed", err)
		}
	})
}
