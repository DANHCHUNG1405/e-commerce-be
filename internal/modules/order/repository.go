package order

import (
	"context"
	"encoding/json"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/payment"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/example/e-commerce-be/internal/modules/voucher"
	"github.com/example/e-commerce-be/internal/outbox"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

type Line struct {
	Variant  models.ProductVariant
	Product  models.Product
	Quantity int
}
type Detail struct {
	Order   models.Order         `json:"order"`
	Sellers []models.SellerOrder `json:"sellerOrders"`
	Items   []models.OrderItem   `json:"items"`
	Payment models.Payment       `json:"payment"`
}

func (r *Repository) Checkout(ctx context.Context, u, address uuid.UUID, key string) (models.Order, error) {
	return r.checkout(ctx, u, address, key, "cod", payment.Config{}, nil, "", "")
}

func (r *Repository) checkout(ctx context.Context, u, address uuid.UUID, key, method string, config payment.Config, selected []uuid.UUID, selection, couponCode string) (models.Order, error) {
	o := models.Order{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// User lock serializes checkout/idempotency; cart lock serializes cart edits.
		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND deleted_at IS NULL", u).First(&user).Error; err != nil {
			return err
		}
		found := tx.Where("user_id=? AND idempotency_key=?", u, key).Limit(1).Find(&o)
		if found.Error != nil {
			return found.Error
		}
		if found.RowsAffected > 0 {
			var previous models.Payment
			if err := tx.Where("order_id=?", o.ID).First(&previous).Error; err != nil {
				return err
			}
			if previous.Method != method || o.CheckoutSelection != selection || o.CouponCode != couponCode {
				return shared.ErrConflict
			}
			if o.ShippingAddressID != address {
				return shared.ErrConflict
			}
			return nil
		}
		var a models.ShippingAddress
		if err := tx.Where("id=? AND user_id=? AND deleted_at IS NULL", address, u).First(&a).Error; err != nil {
			return err
		}
		var cart models.Cart
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id=?", u).First(&cart).Error; err != nil {
			return err
		}
		items := []models.CartItem{}
		query := tx.Where("cart_id=? AND deleted_at IS NULL", cart.ID)
		if len(selected) > 0 {
			query = query.Where("variant_id IN ?", selected)
		}
		if err := query.Order("variant_id").Find(&items).Error; err != nil {
			return err
		}
		if len(items) == 0 || len(items) > 100 || (len(selected) > 0 && len(items) != len(selected)) {
			return shared.ErrConflict
		}
		lines := []Line{}
		var total int64
		for _, item := range items {
			var v models.ProductVariant
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND deleted_at IS NULL", item.VariantID).First(&v).Error; err != nil {
				return err
			}
			var p models.Product
			if err := tx.Where("id=? AND status='published' AND deleted_at IS NULL AND seller_id IN (SELECT id FROM sellers WHERE status='approved' AND deleted_at IS NULL)", v.ProductID).First(&p).Error; err != nil {
				return err
			}
			if item.Quantity < 1 || item.Quantity > 10000 || v.Stock < item.Quantity || v.Price < 0 || v.Price > 1000000000000 {
				return shared.ErrConflict
			}
			total += v.Price * int64(item.Quantity)
			lines = append(lines, Line{v, p, item.Quantity})
		}
		snapshot := map[string]any{}
		body, err := json.Marshal(a)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(body, &snapshot); err != nil {
			return err
		}
		o = models.Order{UserID: u, Status: "pending", Currency: "VND", Subtotal: total, Total: total, ShippingAddressID: address, AddressSnapshot: snapshot, IdempotencyKey: key, CheckoutSelection: selection}
		if err := tx.Create(&o).Error; err != nil {
			return err
		}
		discounts := make([]int64, len(lines))
		if couponCode != "" {
			eligible := make([]voucher.Line, len(lines))
			for i, l := range lines {
				eligible[i] = voucher.Line{SellerID: l.Product.SellerID, Amount: l.Variant.Price * int64(l.Quantity)}
			}
			quote, err := voucher.New(voucher.NewRepository(tx)).Redeem(ctx, u, o.ID, couponCode, eligible)
			if err != nil {
				return err
			}
			discounts = quote.LineDiscounts
			o.CouponCode = couponCode
			o.Discount = quote.Discount
			o.Total = o.Subtotal - o.Discount
			if err := tx.Save(&o).Error; err != nil {
				return err
			}
		}
		groups := map[uuid.UUID]*models.SellerOrder{}
		for i, line := range lines {
			group := groups[line.Product.SellerID]
			if group == nil {
				group = &models.SellerOrder{OrderID: o.ID, SellerID: line.Product.SellerID, Status: "pending"}
				if err := tx.Create(group).Error; err != nil {
					return err
				}
				groups[line.Product.SellerID] = group
			}
			group.Subtotal += line.Variant.Price * int64(line.Quantity)
			group.Discount += discounts[i]
			group.Total = group.Subtotal - group.Discount
			if err := tx.Create(&models.OrderItem{SellerOrderID: group.ID, VariantID: line.Variant.ID, ProductName: line.Product.Name, SKU: line.Variant.SKU, VariantName: line.Variant.Name, UnitPrice: line.Variant.Price, Quantity: line.Quantity, Discount: discounts[i]}).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.ProductVariant{}).Where("id=?", line.Variant.ID).Update("stock", gorm.Expr("stock - ?", line.Quantity)).Error; err != nil {
				return err
			}
			if err := tx.Create(&models.InventoryMovement{VariantID: line.Variant.ID, Quantity: -line.Quantity, Type: "sale", ReferenceID: &o.ID}).Error; err != nil {
				return err
			}
		}
		for _, group := range groups {
			var seller models.Seller
			if err := tx.First(&seller, "id=?", group.SellerID).Error; err != nil {
				return err
			}
			group.Commission = group.Total/10000*int64(seller.CommissionRate) + (group.Total%10000)*int64(seller.CommissionRate)/10000
			if err := tx.Save(group).Error; err != nil {
				return err
			}
		}
		pay := models.Payment{OrderID: o.ID, Method: method, Status: "pending", Amount: o.Total}
		if method == "sepay" {
			if !config.Enabled() || o.Total <= 0 {
				return shared.ErrInvalid
			}
			code := payment.Code(o.ID)
			pay.Code, pay.Bank, pay.AccountNumber = &code, config.Bank, config.Account
		}
		if err := tx.Create(&pay).Error; err != nil {
			return err
		}
		remove := tx.Where("cart_id=?", cart.ID)
		if len(selected) > 0 {
			remove = remove.Where("variant_id IN ?", selected)
		}
		if err := remove.Delete(&models.CartItem{}).Error; err != nil {
			return err
		}
		return outbox.Enqueue(tx, "order", o.ID, "order_created", map[string]any{"orderId": o.ID, "total": o.Total, "discount": o.Discount, "currency": "VND"})
	})
	return o, err
}
func (r *Repository) List(ctx context.Context, u uuid.UUID, p, l int) ([]models.Order, error) {
	v := []models.Order{}
	err := r.db.WithContext(ctx).Where("user_id=? AND deleted_at IS NULL", u).Order("created_at DESC,id").Offset((p - 1) * l).Limit(l).Find(&v).Error
	return v, err
}
func (r *Repository) Detail(ctx context.Context, u, id uuid.UUID) (Detail, error) {
	d := Detail{Sellers: []models.SellerOrder{}, Items: []models.OrderItem{}}
	db := r.db.WithContext(ctx)
	if err := db.Where("id=? AND user_id=? AND deleted_at IS NULL", id, u).First(&d.Order).Error; err != nil {
		return d, err
	}
	if err := db.Where("order_id=?", id).Find(&d.Sellers).Error; err != nil {
		return d, err
	}
	if err := db.Where("seller_order_id IN (SELECT id FROM seller_orders WHERE order_id=?)", id).Find(&d.Items).Error; err != nil {
		return d, err
	}
	err := db.Where("order_id=?", id).First(&d.Payment).Error
	return d, err
}
func (r *Repository) Cancel(ctx context.Context, u, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var o models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND user_id=?", id, u).First(&o).Error; err != nil {
			return err
		}
		if o.Status == "cancelled" {
			return nil
		}
		if o.Status != "pending" {
			return shared.ErrConflict
		}
		var pay models.Payment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_id=?", id).First(&pay).Error; err != nil {
			return err
		}
		// Paid online orders need the refund workflow, never overwrite payment history.
		if pay.Method != "cod" && pay.Status == "paid" {
			return shared.ErrConflict
		}
		var n int64
		if err := tx.Model(&models.SellerOrder{}).Where("order_id=? AND status<>'pending'", id).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return shared.ErrConflict
		}
		items := []models.OrderItem{}
		if err := tx.Where("seller_order_id IN (SELECT id FROM seller_orders WHERE order_id=?)", id).Order("variant_id").Find(&items).Error; err != nil {
			return err
		}
		for _, i := range items {
			if err := tx.Model(&models.ProductVariant{}).Where("id=?", i.VariantID).Update("stock", gorm.Expr("stock + ?", i.Quantity)).Error; err != nil {
				return err
			}
			if err := tx.Create(&models.InventoryMovement{VariantID: i.VariantID, Quantity: i.Quantity, Type: "cancellation", ReferenceID: &id}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&o).Update("status", "cancelled").Error; err != nil {
			return err
		}
		if err := tx.Model(&models.SellerOrder{}).Where("order_id=?", id).Update("status", "cancelled").Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Payment{}).Where("order_id=?", id).Update("status", "cancelled").Error; err != nil {
			return err
		}
		return outbox.Enqueue(tx, "order", id, "order_status_changed", map[string]any{"status": "cancelled"})
	})
}
