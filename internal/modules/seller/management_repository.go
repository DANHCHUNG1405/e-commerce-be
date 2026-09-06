package seller

import (
	"context"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ManagementRepository struct{ db *gorm.DB }

func (r *ManagementRepository) Profile(ctx context.Context, id uuid.UUID) (models.Seller, error) {
	var s models.Seller
	err := r.db.WithContext(ctx).First(&s, "id=? AND deleted_at IS NULL", id).Error
	return s, err
}

func NewManagementRepository(db *gorm.DB) *ManagementRepository { return &ManagementRepository{db} }
func (r *ManagementRepository) Within(ctx context.Context, f func(*ManagementRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return f(NewManagementRepository(tx)) })
}
func (r *ManagementRepository) Role(ctx context.Context, u, id uuid.UUID, lock bool) (string, error) {
	var shop models.Seller
	q := r.db.WithContext(ctx)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := q.First(&shop, "id=? AND deleted_at IS NULL", id).Error; err != nil {
		return "", err
	}
	if err := shared.New(r.db).Seller(ctx, u, id); err != nil {
		return "", err
	}
	var m models.SellerMember
	err := r.db.WithContext(ctx).First(&m, "seller_id=? AND user_id=?", id, u).Error
	return m.Role, err
}
func (r *ManagementRepository) Update(ctx context.Context, id uuid.UUID, in ShopInput) error {
	return r.db.WithContext(ctx).Model(&models.Seller{}).Where("id=?", id).Updates(map[string]any{"name": in.Name, "description": in.Description, "pickup_address": gorm.Expr("?::jsonb", in.pickupJSON())}).Error
}
func (r *ManagementRepository) Members(ctx context.Context, id uuid.UUID, p, l int) ([]models.SellerMember, error) {
	v := []models.SellerMember{}
	err := r.db.WithContext(ctx).Where("seller_id=?", id).Order("user_id").Offset((p - 1) * l).Limit(l).Find(&v).Error
	return v, err
}
func (r *ManagementRepository) Member(ctx context.Context, id, u uuid.UUID) (models.SellerMember, bool, error) {
	var v models.SellerMember
	q := r.db.WithContext(ctx).Where("seller_id=? AND user_id=?", id, u).Limit(1).Find(&v)
	return v, q.RowsAffected > 0, q.Error
}
func (r *ManagementRepository) SetMember(ctx context.Context, id, u uuid.UUID, role string) error {
	if role == "" {
		return r.db.WithContext(ctx).Where("seller_id=? AND user_id=?", id, u).Delete(&models.SellerMember{}).Error
	}
	var n int64
	if err := r.db.WithContext(ctx).Model(&models.User{}).Where("id=? AND deleted_at IS NULL", u).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return gorm.ErrRecordNotFound
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "seller_id"}, {Name: "user_id"}}, DoUpdates: clause.Assignments(map[string]any{"role": role})}).Create(&models.SellerMember{SellerID: id, UserID: u, Role: role}).Error
}
func (r *ManagementRepository) Audit(ctx context.Context, u, id uuid.UUID, kind string, data map[string]any) error {
	// Audit events remain transactionally durable in the existing outbox.
	data["actorId"] = u
	return enqueue(r.db.WithContext(ctx), id, kind, data)
}

type OrderDetail struct {
	Order         models.SellerOrder `json:"sellerOrder"`
	Items         []models.OrderItem `json:"items"`
	Address       map[string]any     `json:"addressSnapshot"`
	Shipment      *models.Shipment   `json:"shipment"`
	PaymentMethod string             `json:"paymentMethod"`
	PaymentStatus string             `json:"paymentStatus"`
}

func (r *ManagementRepository) Detail(ctx context.Context, shop, id uuid.UUID) (OrderDetail, error) {
	d := OrderDetail{Items: []models.OrderItem{}}
	q := r.db.WithContext(ctx)
	if err := q.First(&d.Order, "id=? AND seller_id=? AND deleted_at IS NULL", id, shop).Error; err != nil {
		return d, err
	}
	var o models.Order
	if err := q.First(&o, "id=?", d.Order.OrderID).Error; err != nil {
		return d, err
	}
	d.Address = o.AddressSnapshot
	var p models.Payment
	if err := q.First(&p, "order_id=?", o.ID).Error; err != nil {
		return d, err
	}
	d.PaymentMethod = p.Method
	d.PaymentStatus = p.Status
	if err := q.Where("seller_order_id=?", id).Order("id").Find(&d.Items).Error; err != nil {
		return d, err
	}
	var sh models.Shipment
	result := q.Where("seller_order_id=?", id).Limit(1).Find(&sh)
	if result.RowsAffected > 0 {
		d.Shipment = &sh
	}
	return d, result.Error
}
func (r *ManagementRepository) Orders(ctx context.Context, id uuid.UUID, status string, p, l int) ([]models.SellerOrder, error) {
	v := []models.SellerOrder{}
	q := r.db.WithContext(ctx).Where("seller_id=? AND deleted_at IS NULL", id)
	if status != "" {
		q = q.Where("status=?", status)
	}
	err := q.Order("created_at DESC,id").Offset((p - 1) * l).Limit(l).Find(&v).Error
	return v, err
}

type Dashboard struct {
	Products        int64 `json:"products"`
	PendingOrders   int64 `json:"pendingOrders"`
	DeliveredOrders int64 `json:"deliveredOrders"`
	DeliveredSales  int64 `json:"deliveredSales"`
}

func (r *ManagementRepository) Dashboard(ctx context.Context, id uuid.UUID) (Dashboard, error) {
	var d Dashboard
	q := r.db.WithContext(ctx)
	if err := q.Model(&models.Product{}).Where("seller_id=? AND deleted_at IS NULL", id).Count(&d.Products).Error; err != nil {
		return d, err
	}
	if err := q.Model(&models.SellerOrder{}).Where("seller_id=? AND status='pending'", id).Count(&d.PendingOrders).Error; err != nil {
		return d, err
	}
	if err := q.Model(&models.SellerOrder{}).Where("seller_id=? AND status='delivered'", id).Count(&d.DeliveredOrders).Error; err != nil {
		return d, err
	}
	err := q.Model(&models.SellerOrder{}).Select("COALESCE(SUM(total),0)").Where("seller_id=? AND status='delivered'", id).Scan(&d.DeliveredSales).Error
	return d, err
}
func (r *ManagementRepository) Inventory(ctx context.Context, shop, variant uuid.UUID, p, l int) ([]models.InventoryMovement, error) {
	var n int64
	err := r.db.WithContext(ctx).Table("product_variants v").Joins("JOIN products p ON p.id=v.product_id").Where("v.id=? AND p.seller_id=?", variant, shop).Count(&n).Error
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	v := []models.InventoryMovement{}
	err = r.db.WithContext(ctx).Where("variant_id=?", variant).Order("created_at DESC,id").Offset((p - 1) * l).Limit(l).Find(&v).Error
	return v, err
}
