package order

import (
	"context"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/example/e-commerce-be/internal/outbox"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) SellerOrders(ctx context.Context, u, seller uuid.UUID, p, l int, filters ...string) ([]models.SellerOrder, error) {
	status := ""
	if len(filters) > 0 {
		status = filters[0]
	}
	if p < 1 || p > 100000 || l < 1 || l > 100 {
		return nil, shared.ErrInvalid
	}
	switch status {
	case "", "pending", "confirmed", "shipping", "delivered", "cancelled":
	default:
		return nil, shared.ErrInvalid
	}
	if err := shared.New(s.repo.db).Seller(ctx, u, seller); err != nil {
		return nil, err
	}
	return s.repo.SellerOrders(ctx, seller, p, l, status)
}
func (r *Repository) SellerOrders(ctx context.Context, seller uuid.UUID, p, l int, filters ...string) ([]models.SellerOrder, error) {
	v := []models.SellerOrder{}
	q := r.db.WithContext(ctx).Where("seller_id=?", seller)
	if len(filters) > 0 && filters[0] != "" {
		q = q.Where("status=?", filters[0])
	}
	err := q.Order("created_at DESC,id").Offset((p - 1) * l).Limit(l).Find(&v).Error
	return v, err
}
func validTransition(from, to string) bool {
	return (from == "pending" && to == "confirmed") || (from == "confirmed" && to == "shipping") || (from == "shipping" && to == "delivered")
}
func (s *Service) Fulfill(ctx context.Context, u, seller, id uuid.UUID, status, carrier, tracking string) error {
	if status != "confirmed" && status != "shipping" && status != "delivered" {
		return shared.ErrInvalid
	}
	if status == "shipping" && (carrier == "" || tracking == "") {
		return shared.ErrInvalid
	}
	if err := shared.New(s.repo.db).Seller(ctx, u, seller); err != nil {
		return err
	}
	return s.repo.Fulfill(ctx, seller, id, status, carrier, tracking)
}
func (r *Repository) Fulfill(ctx context.Context, seller, id uuid.UUID, status, carrier, tracking string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var child models.SellerOrder
		if err := tx.Where("id=? AND seller_id=?", id, seller).First(&child).Error; err != nil {
			return err
		}
		var parent models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&parent, "id=?", child.OrderID).Error; err != nil {
			return err
		}
		if err := tx.First(&child, "id=?", id).Error; err != nil {
			return err
		}
		var internalCount int64
		if err := tx.Model(&models.Shipment{}).Where("seller_order_id=? AND carrier='internal'", id).Count(&internalCount).Error; err != nil {
			return err
		}
		if internalCount > 0 {
			return shared.ErrConflict
		}
		if status == "shipping" {
			var n int64
			if err := tx.Model(&models.Shipment{}).Where("seller_order_id IN (SELECT id FROM seller_orders WHERE order_id=?) AND carrier='internal'", parent.ID).Count(&n).Error; err != nil {
				return err
			}
			if n > 0 {
				return shared.ErrConflict
			}
		}
		if child.Status == status {
			return nil
		}
		if !validTransition(child.Status, status) || parent.Status == "cancelled" {
			return shared.ErrConflict
		}
		var pay models.Payment
		if err := tx.Where("order_id=?", parent.ID).First(&pay).Error; err != nil {
			return err
		}
		if pay.Method != "cod" && pay.Status != "paid" {
			return shared.ErrConflict
		}
		if err := tx.Model(&child).Update("status", status).Error; err != nil {
			return err
		}
		if status == "shipping" {
			shipment := models.Shipment{SellerOrderID: id, Carrier: carrier, TrackingNumber: tracking, Status: status}
			if err := tx.Create(&shipment).Error; err != nil {
				return err
			}
			if err := tx.Create(&models.ShipmentEvent{ShipmentID: shipment.ID, Status: status}).Error; err != nil {
				return err
			}
		}
		if status == "delivered" {
			var shipment models.Shipment
			if err := tx.Where("seller_order_id=?", id).First(&shipment).Error; err != nil {
				return err
			}
			if err := tx.Model(&shipment).Update("status", status).Error; err != nil {
				return err
			}
			if err := tx.Create(&models.ShipmentEvent{ShipmentID: shipment.ID, Status: status}).Error; err != nil {
				return err
			}
		}
		children := []models.SellerOrder{}
		if err := tx.Where("order_id=?", parent.ID).Find(&children).Error; err != nil {
			return err
		}
		allDelivered := true
		for _, c := range children {
			if c.Status != "delivered" {
				allDelivered = false
			}
		}
		parentStatus := "processing"
		if allDelivered {
			parentStatus = "delivered"
			if err := tx.Model(&models.Payment{}).Where("order_id=? AND method='cod' AND NOT EXISTS (SELECT 1 FROM shipments s JOIN seller_orders so ON so.id=s.seller_order_id WHERE so.order_id=? AND s.carrier='internal')", parent.ID, parent.ID).Update("status", "paid").Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&parent).Update("status", parentStatus).Error; err != nil {
			return err
		}
		return outbox.Enqueue(tx, "seller_order", id, "order_status_changed", map[string]any{"status": status})
	})
}
