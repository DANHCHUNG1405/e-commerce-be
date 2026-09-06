package payment_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/example/e-commerce-be/internal/database"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/order"
	"github.com/example/e-commerce-be/internal/modules/payment"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestSePayIntegration(t *testing.T) {
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
	cfg := payment.Config{Bank: "TestBank", Account: "test-account", WebhookSecret: strings.Repeat("x", 32)}
	service := payment.New(payment.NewRepository(db), cfg)
	orders := order.New(order.NewRepository(db), cfg)
	ctx := context.Background()
	create := func(v any) {
		t.Helper()
		if err := db.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	fixture := func() (models.User, models.Order, models.Payment) {
		t.Helper()
		u := models.User{Email: uuid.NewString() + "@example.test", Password: "unused"}
		create(&u)
		a := models.ShippingAddress{UserID: u.ID, Country: "VN"}
		create(&a)
		shop := models.Seller{Name: "shop", Slug: uuid.NewString(), Status: "approved"}
		create(&shop)
		create(&models.SellerMember{SellerID: shop.ID, UserID: u.ID, Role: "owner"})
		p := models.Product{SellerID: shop.ID, Name: "product", Slug: uuid.NewString(), Status: "published"}
		create(&p)
		v := models.ProductVariant{ProductID: p.ID, Name: "variant", SKU: uuid.NewString(), Price: 1000, Stock: 1, Attributes: map[string]any{}}
		create(&v)
		cart := models.Cart{UserID: u.ID}
		create(&cart)
		create(&models.CartItem{CartID: cart.ID, VariantID: v.ID, Quantity: 1})
		o, err := orders.Checkout(ctx, u.ID, a.ID, uuid.NewString(), "sepay")
		if err != nil {
			t.Fatal(err)
		}
		d, err := orders.Detail(ctx, u.ID, o.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := orders.Fulfill(ctx, u.ID, shop.ID, d.Sellers[0].ID, "confirmed", "", ""); !errors.Is(err, shared.ErrConflict) {
			t.Fatal("unpaid order can be fulfilled")
		}
		return u, o, d.Payment
	}
	webhook := func(p models.Payment) payment.Webhook {
		return payment.Webhook{ID: int64(uuid.New().ID()) + 1, Gateway: cfg.Bank, AccountNumber: cfg.Account, Content: *p.Code, TransferType: "in", TransferAmount: p.Amount}
	}
	t.Run("duplicate concurrent delivery", func(t *testing.T) {
		u, o, p := fixture()
		w := webhook(p)
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); errs <- service.Receive(ctx, w) }()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		var count int64
		if err := db.Model(&models.PaymentTransaction{}).Where("payment_id=?", p.ID).Count(&count).Error; err != nil || count != 1 {
			t.Fatal("duplicate transaction", err)
		}
		if err := db.Model(&models.OutboxEvent{}).Where("aggregate_id=? AND event_type='payment_succeeded'", p.ID).Count(&count).Error; err != nil || count != 1 {
			t.Fatal("duplicate success event", err)
		}
		if err := orders.Cancel(ctx, u.ID, o.ID); !errors.Is(err, shared.ErrConflict) {
			t.Fatal("paid online order cancelled without refund")
		}
		if _, err := service.Instructions(ctx, uuid.New(), o.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatal("another user accessed payment")
		}
		info, err := service.Instructions(ctx, u.ID, o.ID)
		if err != nil || info.Status != "paid" || info.QRURL != "" {
			t.Fatal("invalid paid instructions")
		}
		w.TransferAmount++
		if err := service.Receive(ctx, w); !errors.Is(err, shared.ErrConflict) {
			t.Fatal("mutated duplicate accepted")
		}
	})
	for _, scenario := range []string{"underpaid", "wrong account", "wrong bank", "cancelled", "outgoing", "unknown code"} {
		t.Run(scenario, func(t *testing.T) {
			u, o, p := fixture()
			w := webhook(p)
			switch scenario {
			case "underpaid":
				w.TransferAmount--
			case "wrong account":
				w.AccountNumber = "other"
			case "wrong bank":
				w.Gateway = "other"
			case "cancelled":
				if err := orders.Cancel(ctx, u.ID, o.ID); err != nil {
					t.Fatal(err)
				}
			case "outgoing":
				w.TransferType = "out"
			case "unknown code":
				w.Content = payment.Code(uuid.New())
			}
			if err := service.Receive(ctx, w); err != nil {
				t.Fatal(err)
			}
			if err := db.First(&p, "id=?", p.ID).Error; err != nil {
				t.Fatal(err)
			}
			if p.Status == "paid" {
				t.Fatal("invalid transfer credited")
			}
		})
	}
}
