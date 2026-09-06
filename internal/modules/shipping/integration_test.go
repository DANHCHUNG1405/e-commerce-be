package shipping

import (
	"context"
	"errors"
	"github.com/example/e-commerce-be/internal/database"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/order"
	"github.com/example/e-commerce-be/internal/modules/seller"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
	"os"
	"sync"
	"testing"
)

func TestInternalDeliveryIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable PostgreSQL container; never the application database")
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
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	create := func(v any) {
		t.Helper()
		if err := db.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	users := make([]models.User, 5)
	for i := range users {
		users[i] = models.User{Email: uuid.NewString() + "@example.test", Password: "unused"}
		create(&users[i])
	}
	buyer, owner, driver, admin, outsider := users[0].ID, users[1].ID, users[2].ID, users[3].ID, users[4].ID
	var role models.Role
	if err := db.First(&role, "name='admin'").Error; err != nil {
		t.Fatal(err)
	}
	create(&models.UserRole{UserID: admin, RoleID: role.ID})
	shop := models.Seller{Name: "Shop", Slug: uuid.NewString(), Status: "approved"}
	create(&shop)
	create(&models.SellerMember{SellerID: shop.ID, UserID: owner, Role: "owner"})
	ctx := context.Background()
	management := seller.NewManagement(seller.NewManagementRepository(db))
	input := seller.ShopInput{Name: "Shop", PickupAddress: seller.PickupAddress{RecipientName: "Owner", Phone: "0900000000", AddressLine: "Original pickup", Ward: "Ward", District: "District", Province: "City"}}
	if err := management.Update(ctx, owner, shop.ID, input); err != nil {
		t.Fatal(err)
	}
	if err := management.SetMember(ctx, owner, shop.ID, owner, ""); !errors.Is(err, shared.ErrConflict) {
		t.Fatal("owner removal allowed", err)
	}
	if err := management.SetMember(ctx, owner, shop.ID, outsider, "staff"); err != nil {
		t.Fatal(err)
	}
	if err := management.SetMember(ctx, outsider, shop.ID, driver, "manager"); !errors.Is(err, shared.ErrForbidden) {
		t.Fatal("staff escalated", err)
	}
	if err := management.SetMember(ctx, owner, shop.ID, outsider, ""); err != nil {
		t.Fatal(err)
	}
	address := models.ShippingAddress{UserID: buyer, RecipientName: "Buyer", AddressLine: "Original buyer", Country: "VN"}
	create(&address)
	product := models.Product{SellerID: shop.ID, Name: "Product", Slug: uuid.NewString(), Status: "published"}
	create(&product)
	variant := models.ProductVariant{ProductID: product.ID, Name: "Variant", SKU: uuid.NewString(), Price: 100, Stock: 3, Attributes: map[string]any{}}
	create(&variant)
	cart := models.Cart{UserID: buyer}
	create(&cart)
	create(&models.CartItem{CartID: cart.ID, VariantID: variant.ID, Quantity: 1})
	orders := order.New(order.NewRepository(db))
	o, err := orders.Checkout(ctx, buyer, address.ID, uuid.NewString(), "cod")
	if err != nil {
		t.Fatal(err)
	}
	var child models.SellerOrder
	if err := db.First(&child, "order_id=?", o.ID).Error; err != nil {
		t.Fatal(err)
	}
	s := New(NewRepository(db))
	if _, err := s.Create(ctx, owner, shop.ID, child.ID); !errors.Is(err, shared.ErrConflict) {
		t.Fatal("unconfirmed shipment", err)
	}
	if err := orders.Fulfill(ctx, owner, shop.ID, child.ID, "confirmed", "", ""); err != nil {
		t.Fatal(err)
	}
	shipment, err := s.Create(ctx, owner, shop.ID, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Create(ctx, owner, shop.ID, child.ID)
	if err != nil || again.ID != shipment.ID {
		t.Fatal("duplicate shipment", err)
	}
	if err := orders.Fulfill(ctx, owner, shop.ID, child.ID, "shipping", "fake", "fake"); !errors.Is(err, shared.ErrConflict) {
		t.Fatal("seller bypassed driver", err)
	}
	if _, err := s.Apply(ctx, driver, Application{Phone: "0900000000", VehiclePlate: "TEST-123"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Assign(ctx, admin, shipment.ID, driver); !errors.Is(err, shared.ErrForbidden) {
		t.Fatal("unapproved driver assigned", err)
	}
	if err := s.Approve(ctx, buyer, driver, "approved"); !errors.Is(err, shared.ErrForbidden) {
		t.Fatal("buyer approved driver", err)
	}
	if err := s.Approve(ctx, admin, driver, "approved"); err != nil {
		t.Fatal(err)
	}
	if err := s.Assign(ctx, admin, shipment.ID, driver); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Detail(ctx, outsider, shipment.ID); !errors.Is(err, shared.ErrForbidden) {
		t.Fatal("outsider accessed address", err)
	}
	if _, err := management.Detail(ctx, outsider, shop.ID, child.ID); !errors.Is(err, shared.ErrForbidden) {
		t.Fatal("revoked member accessed order", err)
	}
	input.PickupAddress.AddressLine = "Changed"
	if err := management.Update(ctx, owner, shop.ID, input); err != nil {
		t.Fatal(err)
	}
	detail, err := s.Detail(ctx, buyer, shipment.ID)
	if err != nil || detail.PickupSnapshot["addressLine"] != "Original pickup" {
		t.Fatal("snapshot changed", err)
	}
	if err := s.Update(ctx, driver, shipment.ID, "skip", Update{Status: "delivered", CODCollected: true}); !errors.Is(err, shared.ErrConflict) {
		t.Fatal("skipped states", err)
	}
	for _, status := range []string{"accepted", "picked_up", "delivering", "failed", "delivering"} {
		in := Update{Status: status}
		if status == "failed" {
			in.Reason = "recipient unavailable"
		}
		if err := s.Update(ctx, driver, shipment.ID, uuid.NewString(), in); err != nil {
			t.Fatal(status, err)
		}
	}
	if err := s.Assign(ctx, admin, shipment.ID, driver); !errors.Is(err, shared.ErrConflict) {
		t.Fatal("reassigned after pickup", err)
	}
	if err := s.Update(ctx, driver, shipment.ID, "no-cash", Update{Status: "delivered"}); !errors.Is(err, shared.ErrInvalid) {
		t.Fatal("COD delivery without collection", err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- s.Update(ctx, driver, shipment.ID, "delivered-key", Update{Status: "delivered", CODCollected: true})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	if err := db.Model(&models.ShipmentEvent{}).Where("shipment_id=? AND request_id='delivered-key'", shipment.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("duplicate delivery event", count, err)
	}
	var pay models.Payment
	if err := db.First(&pay, "order_id=?", o.ID).Error; err != nil {
		t.Fatal(err)
	}
	if pay.Status != "pending" {
		t.Fatal("collection marked settled")
	}
	if err := s.Settle(ctx, driver, shipment.ID); !errors.Is(err, shared.ErrForbidden) {
		t.Fatal("driver settled cash", err)
	}
	for i := 0; i < 2; i++ {
		if err := s.Settle(ctx, admin, shipment.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.First(&pay, "order_id=?", o.ID).Error; err != nil {
		t.Fatal(err)
	}
	if pay.Status != "paid" {
		t.Fatal("settlement not applied")
	}
	if err := db.Model(&models.OutboxEvent{}).Where("aggregate_id=? AND event_type='payment_succeeded'", o.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("duplicate payment event", count, err)
	}
}
