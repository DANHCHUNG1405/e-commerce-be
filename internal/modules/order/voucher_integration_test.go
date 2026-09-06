package order

import (
	"context"
	"github.com/example/e-commerce-be/internal/database"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/google/uuid"
	"os"
	"sync"
	"testing"
	"time"
)

func TestVoucherCheckoutIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable PostgreSQL container")
	}
	db, err := database.Connect(dsn)
	if err != nil {
		t.Fatal("test database unavailable")
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	create := func(v any) {
		t.Helper()
		if err := db.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	shop := models.Seller{Name: "shop", Slug: uuid.NewString(), Status: "approved", CommissionRate: 1000}
	create(&shop)
	product := models.Product{SellerID: shop.ID, Name: "product", Slug: uuid.NewString(), Status: "published"}
	create(&product)
	variant := models.ProductVariant{ProductID: product.ID, Name: "variant", SKU: uuid.NewString(), Price: 1000, Stock: 10, Attributes: map[string]any{}}
	create(&variant)
	type buyer struct{ u, a, c uuid.UUID }
	buyers := []buyer{}
	for i := 0; i < 2; i++ {
		u := models.User{Email: uuid.NewString() + "@example.test", Password: "unused"}
		create(&u)
		a := models.ShippingAddress{UserID: u.ID, Country: "VN"}
		create(&a)
		c := models.Cart{UserID: u.ID}
		create(&c)
		create(&models.CartItem{CartID: c.ID, VariantID: variant.ID, Quantity: 1})
		buyers = append(buyers, buyer{u.ID, a.ID, c.ID})
	}
	coupon := models.Coupon{Code: uuid.NewString(), Type: "fixed", Value: 333, Active: true, UsageLimit: 1, StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now().Add(time.Hour)}
	// Service normalizes code to uppercase.
	coupon.Code = "V" + uuid.New().String()[:8]
	for i, b := range []byte(coupon.Code) {
		if b >= 'a' && b <= 'z' {
			v := []byte(coupon.Code)
			v[i] = b - 32
			coupon.Code = string(v)
		}
	}
	create(&coupon)
	create(&models.CouponRule{CouponID: coupon.ID, MinOrder: 100, SellerID: &shop.ID})
	s := New(NewRepository(db))
	ctx := context.Background()
	type result struct {
		o     models.Order
		err   error
		buyer buyer
		key   string
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, b := range buyers {
		wg.Add(1)
		go func(b buyer) {
			defer wg.Done()
			key := uuid.NewString()
			o, err := s.CheckoutVoucher(ctx, b.u, b.a, key, "cod", nil, coupon.Code)
			results <- result{o, err, b, key}
		}(b)
	}
	wg.Wait()
	close(results)
	wins := 0
	var winner result
	for r := range results {
		if r.err == nil {
			wins++
			winner = r
		}
	}
	if wins != 1 {
		t.Fatalf("expected one voucher winner, got %d", wins)
	}
	d, err := s.Detail(ctx, winner.buyer.u, winner.o.ID)
	if err != nil || d.Order.Discount != 333 || d.Order.Total != 667 || d.Payment.Amount != 667 || d.Sellers[0].Total != 667 || d.Sellers[0].Commission != 66 || d.Items[0].Discount != 333 {
		t.Fatal("discount snapshots inconsistent", err)
	}
	if err := db.First(&variant, "id=?", variant.ID).Error; err != nil || variant.Stock != 9 {
		t.Fatal("failed checkout changed stock", err)
	}
	if err := db.First(&coupon, "id=?", coupon.ID).Error; err != nil || coupon.UsedCount != 1 {
		t.Fatal("quota exceeded", err)
	}
	repeat, err := s.CheckoutVoucher(ctx, winner.buyer.u, winner.buyer.a, winner.key, "cod", nil, coupon.Code)
	if err != nil || repeat.ID != winner.o.ID {
		t.Fatal("retry consumed coupon again", err)
	}
	if _, err := s.CheckoutVoucher(ctx, winner.buyer.u, winner.buyer.a, winner.key, "cod", nil, ""); err == nil {
		t.Fatal("changed coupon accepted with same key")
	}
	if err := s.Cancel(ctx, winner.buyer.u, winner.o.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&coupon, "id=?", coupon.ID).Error; err != nil || coupon.UsedCount != 1 {
		t.Fatal("cancellation changed documented quota policy", err)
	}
}
