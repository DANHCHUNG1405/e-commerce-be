package shipping

import (
	"context"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/example/e-commerce-be/internal/outbox"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct{ db *gorm.DB }

func (r *Repository) HasExternal(ctx context.Context, order uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.Shipment{}).Where("seller_order_id IN (SELECT id FROM seller_orders WHERE order_id=?) AND carrier<>'internal'", order).Count(&n).Error
	return n > 0, err
}

func (r *Repository) orderView(ctx context.Context, id uuid.UUID, o *models.Order) error {
	return r.db.WithContext(ctx).First(o, "id=?", id).Error
}
func NewRepository(db *gorm.DB) *Repository { return &Repository{db} }
func (r *Repository) Within(ctx context.Context, f func(*Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return f(NewRepository(tx)) })
}
func (r *Repository) Admin(ctx context.Context, u uuid.UUID) error {
	return shared.New(r.db).Admin(ctx, u)
}
func (r *Repository) Seller(ctx context.Context, u, s uuid.UUID) error {
	return shared.New(r.db).Seller(ctx, u, s)
}
func (r *Repository) Active(ctx context.Context, u uuid.UUID) error {
	var n int64
	if err := r.db.WithContext(ctx).Model(&models.User{}).Where("id=? AND deleted_at IS NULL", u).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return shared.ErrForbidden
	}
	return nil
}
func (r *Repository) Profile(ctx context.Context, u uuid.UUID, lock bool) (models.DriverProfile, error) {
	var d models.DriverProfile
	q := r.db.WithContext(ctx)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.First(&d, "user_id=?", u).Error
	return d, err
}
func (r *Repository) Apply(ctx context.Context, d *models.DriverProfile) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(d).Error
}
func (r *Repository) Drivers(ctx context.Context, status string, p, l int) ([]models.DriverProfile, error) {
	v := []models.DriverProfile{}
	q := r.db.WithContext(ctx)
	if status != "" {
		q = q.Where("status=?", status)
	}
	err := q.Order("created_at DESC,user_id").Offset((p - 1) * l).Limit(l).Find(&v).Error
	return v, err
}
func (r *Repository) DriverStatus(ctx context.Context, u uuid.UUID, status string) error {
	if err := r.db.WithContext(ctx).Model(&models.DriverProfile{}).Where("user_id=?", u).Update("status", status).Error; err != nil {
		return err
	}
	if status == "approved" {
		return r.db.WithContext(ctx).Exec("INSERT INTO user_roles(user_id,role_id) SELECT ?,id FROM roles WHERE name='driver' ON CONFLICT DO NOTHING", u).Error
	}
	return nil // Authorization always checks live profile status, never the JWT role alone.
}
func (r *Repository) Child(ctx context.Context, id uuid.UUID) (models.SellerOrder, error) {
	var c models.SellerOrder
	err := r.db.WithContext(ctx).First(&c, "id=? AND deleted_at IS NULL", id).Error
	return c, err
}
func (r *Repository) LockOrder(ctx context.Context, id uuid.UUID) (models.Order, error) {
	var o models.Order
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&o, "id=?", id).Error
	return o, err
}
func (r *Repository) Payment(ctx context.Context, id uuid.UUID) (models.Payment, error) {
	var p models.Payment
	err := r.db.WithContext(ctx).First(&p, "order_id=?", id).Error
	return p, err
}
func (r *Repository) Shop(ctx context.Context, id uuid.UUID) (models.Seller, error) {
	var s models.Seller
	err := r.db.WithContext(ctx).First(&s, "id=? AND deleted_at IS NULL", id).Error
	return s, err
}
func (r *Repository) ByChild(ctx context.Context, id uuid.UUID) (models.Shipment, bool, error) {
	var s models.Shipment
	q := r.db.WithContext(ctx).Where("seller_order_id=?", id).Limit(1).Find(&s)
	return s, q.RowsAffected > 0, q.Error
}
func (r *Repository) Shipment(ctx context.Context, id uuid.UUID, lock bool) (models.Shipment, error) {
	var s models.Shipment
	q := r.db.WithContext(ctx)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.First(&s, "id=? AND carrier='internal' AND deleted_at IS NULL", id).Error
	return s, err
}
func (r *Repository) Create(ctx context.Context, s *models.Shipment) error {
	return r.db.WithContext(ctx).Create(s).Error
}
func (r *Repository) Save(ctx context.Context, s *models.Shipment) error {
	return r.db.WithContext(ctx).Save(s).Error
}
func (r *Repository) Event(ctx context.Context, s models.Shipment, u uuid.UUID, key, description string) error {
	e := models.ShipmentEvent{ShipmentID: s.ID, Status: s.Status, Description: description, ActorID: &u}
	if key != "" {
		e.RequestID = &key
	}
	if err := r.db.WithContext(ctx).Create(&e).Error; err != nil {
		return err
	}
	return outbox.Enqueue(r.db.WithContext(ctx), "shipment", s.ID, "shipment_status_changed", map[string]any{"status": s.Status, "sellerOrderId": s.SellerOrderID, "driverId": s.DriverID})
}
func (r *Repository) Prior(ctx context.Context, id uuid.UUID, key string) (models.ShipmentEvent, bool, error) {
	var e models.ShipmentEvent
	q := r.db.WithContext(ctx).Where("shipment_id=? AND request_id=?", id, key).Limit(1).Find(&e)
	return e, q.RowsAffected > 0, q.Error
}
func (r *Repository) Events(ctx context.Context, id uuid.UUID, p, l int) ([]models.ShipmentEvent, error) {
	v := []models.ShipmentEvent{}
	err := r.db.WithContext(ctx).Where("shipment_id=?", id).Order("created_at,id").Offset((p - 1) * l).Limit(l).Find(&v).Error
	return v, err
}
func (r *Repository) List(ctx context.Context, u *uuid.UUID, status string, p, l int) ([]models.Shipment, error) {
	v := []models.Shipment{}
	q := r.db.WithContext(ctx).Where("carrier='internal' AND deleted_at IS NULL")
	if u != nil {
		q = q.Where("driver_id=?", *u)
	}
	if status != "" {
		q = q.Where("status=?", status)
	}
	err := q.Order("created_at DESC,id").Offset((p - 1) * l).Limit(l).Find(&v).Error
	return v, err
}
func (r *Repository) SyncOrder(ctx context.Context, c models.SellerOrder, status string) error {
	if err := r.db.WithContext(ctx).Model(&c).Update("status", status).Error; err != nil {
		return err
	}
	var n int64
	if err := r.db.WithContext(ctx).Model(&models.SellerOrder{}).Where("order_id=? AND status<>'delivered'", c.OrderID).Count(&n).Error; err != nil {
		return err
	}
	parent := "processing"
	if n == 0 {
		parent = "delivered"
	}
	// Delivery and COD settlement are deliberately separate.
	return r.db.WithContext(ctx).Model(&models.Order{}).Where("id=?", c.OrderID).Update("status", parent).Error
}
func (r *Repository) SettlePayment(ctx context.Context, o models.Order) error {
	var n int64
	if err := r.db.WithContext(ctx).Model(&models.SellerOrder{}).Where("order_id=? AND (status<>'delivered' OR NOT EXISTS (SELECT 1 FROM shipments s WHERE s.seller_order_id=seller_orders.id AND s.carrier='internal' AND s.cod_settled))", o.ID).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	result := r.db.WithContext(ctx).Model(&models.Payment{}).Where("order_id=? AND method='cod' AND status='pending'", o.ID).Update("status", "paid")
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return nil
	}
	return outbox.Enqueue(r.db.WithContext(ctx), "order", o.ID, "payment_succeeded", map[string]any{"method": "cod"})
}
