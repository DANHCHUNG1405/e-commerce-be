package shipping

import (
	"context"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
	"strings"
)

type Service struct{ repo *Repository }

func New(r *Repository) *Service { return &Service{r} }
func page(p, l int) bool         { return p >= 1 && p <= 100000 && l >= 1 && l <= 100 }
func driverStatus(v string) bool {
	return v == "pending" || v == "approved" || v == "rejected" || v == "suspended"
}
func shipmentStatus(v string) bool {
	switch v {
	case "pending", "assigned", "accepted", "picked_up", "delivering", "failed", "returned", "delivered":
		return true
	}
	return false
}
func Transition(from, to string) bool {
	switch from {
	case "assigned":
		return to == "accepted"
	case "accepted":
		return to == "picked_up"
	case "picked_up":
		return to == "delivering"
	case "delivering":
		return to == "delivered" || to == "failed"
	case "failed":
		return to == "delivering" || to == "returned"
	}
	return false
}

type Application struct {
	Phone        string `json:"phone"`
	VehiclePlate string `json:"vehiclePlate"`
}

func (s *Service) Apply(ctx context.Context, u uuid.UUID, in Application) (models.DriverProfile, error) {
	d := models.DriverProfile{UserID: u, Phone: strings.TrimSpace(in.Phone), VehiclePlate: strings.TrimSpace(in.VehiclePlate), Status: "pending"}
	if len(d.Phone) < 8 || len(d.Phone) > 20 || len(d.VehiclePlate) < 3 || len(d.VehiclePlate) > 30 {
		return d, shared.ErrInvalid
	}
	if err := s.repo.Active(ctx, u); err != nil {
		return d, err
	}
	if err := s.repo.Apply(ctx, &d); err != nil {
		return d, err
	}
	return s.repo.Profile(ctx, u, false)
}
func (s *Service) Me(ctx context.Context, u uuid.UUID) (models.DriverProfile, error) {
	if err := s.repo.Active(ctx, u); err != nil {
		return models.DriverProfile{}, err
	}
	return s.repo.Profile(ctx, u, false)
}
func (s *Service) Drivers(ctx context.Context, u uuid.UUID, status string, p, l int) ([]models.DriverProfile, error) {
	if !page(p, l) || (status != "" && !driverStatus(status)) {
		return nil, shared.ErrInvalid
	}
	if err := s.repo.Admin(ctx, u); err != nil {
		return nil, err
	}
	return s.repo.Drivers(ctx, status, p, l)
}
func (s *Service) Approve(ctx context.Context, u, id uuid.UUID, status string) error {
	if status != "approved" && status != "rejected" && status != "suspended" {
		return shared.ErrInvalid
	}
	return s.repo.Within(ctx, func(r *Repository) error {
		if err := r.Admin(ctx, u); err != nil {
			return err
		}
		if err := r.Active(ctx, id); err != nil {
			return err
		}
		if _, err := r.Profile(ctx, id, true); err != nil {
			return err
		}
		return r.DriverStatus(ctx, id, status)
	})
}
func approved(ctx context.Context, r *Repository, u uuid.UUID) error {
	if err := r.Active(ctx, u); err != nil {
		return err
	}
	d, err := r.Profile(ctx, u, true)
	if err != nil {
		return err
	}
	if d.Status != "approved" {
		return shared.ErrForbidden
	}
	return nil
}

// Every write takes the parent order lock first, matching checkout/payment/cancellation.
func (s *Service) mutate(ctx context.Context, id uuid.UUID, f func(*Repository, *models.Shipment, models.SellerOrder, models.Order) error) error {
	return s.repo.Within(ctx, func(r *Repository) error {
		before, err := r.Shipment(ctx, id, false)
		if err != nil {
			return err
		}
		c, err := r.Child(ctx, before.SellerOrderID)
		if err != nil {
			return err
		}
		o, err := r.LockOrder(ctx, c.OrderID)
		if err != nil {
			return err
		}
		v, err := r.Shipment(ctx, id, true)
		if err != nil {
			return err
		}
		c, err = r.Child(ctx, c.ID)
		if err != nil {
			return err
		}
		if o.Status == "cancelled" {
			return shared.ErrConflict
		}
		return f(r, &v, c, o)
	})
}
func (s *Service) Create(ctx context.Context, u, shop, id uuid.UUID) (models.Shipment, error) {
	var v models.Shipment
	err := s.repo.Within(ctx, func(r *Repository) error {
		if err := r.Seller(ctx, u, shop); err != nil {
			return err
		}
		c, err := r.Child(ctx, id)
		if err != nil {
			return err
		}
		if c.SellerID != shop {
			return shared.ErrForbidden
		}
		o, err := r.LockOrder(ctx, c.OrderID)
		if err != nil {
			return err
		}
		c, err = r.Child(ctx, id)
		if err != nil {
			return err
		}
		old, found, err := r.ByChild(ctx, id)
		if err != nil {
			return err
		}
		if found {
			if old.Carrier != "internal" {
				return shared.ErrConflict
			}
			v = old
			return nil
		}
		if c.Status != "confirmed" || o.Status == "cancelled" {
			return shared.ErrConflict
		}
		mixed, err := r.HasExternal(ctx, o.ID)
		if err != nil {
			return err
		}
		if mixed {
			return shared.ErrConflict
		}
		pay, err := r.Payment(ctx, o.ID)
		if err != nil {
			return err
		}
		if pay.Method != "cod" && pay.Status != "paid" {
			return shared.ErrConflict
		}
		seller, err := r.Shop(ctx, shop)
		if err != nil {
			return err
		}
		if seller.Status != "approved" || len(seller.PickupAddress) == 0 {
			return shared.ErrConflict
		}
		v = models.Shipment{SellerOrderID: id, Carrier: "internal", TrackingNumber: "INT" + strings.ReplaceAll(uuid.NewString(), "-", ""), Status: "pending", AddressSnapshot: o.AddressSnapshot, PickupSnapshot: seller.PickupAddress}
		if pay.Method == "cod" {
			v.CODAmount = c.Total
		}
		// Current checkout has zero shipping fee. Reject ambiguous COD allocation instead of under-collecting.
		if pay.Method == "cod" && o.ShippingFee != 0 {
			return shared.ErrConflict
		}
		if err := r.Create(ctx, &v); err != nil {
			return err
		}
		return r.Event(ctx, v, u, "", "delivery requested")
	})
	return v, err
}
func (s *Service) Assign(ctx context.Context, u, id, driver uuid.UUID) error {
	if driver == uuid.Nil {
		return shared.ErrInvalid
	}
	return s.mutate(ctx, id, func(r *Repository, v *models.Shipment, c models.SellerOrder, o models.Order) error {
		if err := r.Admin(ctx, u); err != nil {
			return err
		}
		if err := approved(ctx, r, driver); err != nil {
			return err
		}
		if v.Status != "pending" && v.Status != "assigned" && v.Status != "accepted" {
			return shared.ErrConflict
		}
		if v.DriverID != nil && *v.DriverID == driver {
			return nil
		}
		v.DriverID = &driver
		v.Status = "assigned"
		if err := r.Save(ctx, v); err != nil {
			return err
		}
		return r.Event(ctx, *v, u, "", "assigned to "+driver.String())
	})
}

type Update struct {
	Status       string `json:"status"`
	Reason       string `json:"reason"`
	CODCollected bool   `json:"codCollected"`
}

func (s *Service) Update(ctx context.Context, u, id uuid.UUID, key string, in Update) error {
	if !shipmentStatus(in.Status) || strings.TrimSpace(key) == "" || len(key) > 128 || len(in.Reason) > 1000 || (in.Status == "failed" && strings.TrimSpace(in.Reason) == "") || (in.CODCollected && in.Status != "delivered") {
		return shared.ErrInvalid
	}
	return s.mutate(ctx, id, func(r *Repository, v *models.Shipment, c models.SellerOrder, o models.Order) error {
		if err := approved(ctx, r, u); err != nil {
			return err
		}
		if v.DriverID == nil || *v.DriverID != u {
			return shared.ErrForbidden
		}
		old, found, err := r.Prior(ctx, id, key)
		if err != nil {
			return err
		}
		if found {
			if old.ActorID == nil || *old.ActorID != u || old.Status != in.Status || old.Description != in.Reason {
				return shared.ErrConflict
			}
			if in.Status == "delivered" && (v.CODAmount > 0) != in.CODCollected {
				return shared.ErrConflict
			}
			return nil
		}
		if !Transition(v.Status, in.Status) {
			return shared.ErrConflict
		}
		pay, err := r.Payment(ctx, o.ID)
		if err != nil {
			return err
		}
		if pay.Method != "cod" && pay.Status != "paid" {
			return shared.ErrConflict
		}
		if in.Status == "delivered" {
			if (v.CODAmount > 0) != in.CODCollected {
				return shared.ErrInvalid
			}
			v.CODCollected = in.CODCollected
		}
		v.Status = in.Status
		if err := r.Save(ctx, v); err != nil {
			return err
		}
		childStatus := "shipping"
		if in.Status == "accepted" {
			childStatus = "confirmed"
		}
		if in.Status == "delivered" {
			childStatus = "delivered"
		}
		if err := r.SyncOrder(ctx, c, childStatus); err != nil {
			return err
		}
		return r.Event(ctx, *v, u, key, in.Reason)
	})
}
func (s *Service) Settle(ctx context.Context, u, id uuid.UUID) error {
	return s.mutate(ctx, id, func(r *Repository, v *models.Shipment, c models.SellerOrder, o models.Order) error {
		if err := r.Admin(ctx, u); err != nil {
			return err
		}
		pay, err := r.Payment(ctx, o.ID)
		if err != nil {
			return err
		}
		if pay.Method != "cod" || v.Status != "delivered" || (!v.CODCollected && v.CODAmount > 0) {
			return shared.ErrConflict
		}
		if v.CODSettled {
			return nil
		}
		v.CODSettled = true
		v.CODCollected = true // Zero-value COD needs no cash; settlement still requires admin acknowledgement.
		if err := r.Save(ctx, v); err != nil {
			return err
		}
		if err := r.Event(ctx, *v, u, "", "COD settled"); err != nil {
			return err
		}
		return r.SettlePayment(ctx, o)
	})
}
func (s *Service) List(ctx context.Context, u uuid.UUID, admin bool, status string, p, l int) ([]models.Shipment, error) {
	if !page(p, l) || (status != "" && !shipmentStatus(status)) {
		return nil, shared.ErrInvalid
	}
	if admin {
		if err := s.repo.Admin(ctx, u); err != nil {
			return nil, err
		}
		return s.repo.List(ctx, nil, status, p, l)
	}
	d, err := s.Me(ctx, u)
	if err != nil {
		return nil, err
	}
	if d.Status != "approved" {
		return nil, shared.ErrForbidden
	}
	return s.repo.List(ctx, &u, status, p, l)
}
func (s *Service) Detail(ctx context.Context, u, id uuid.UUID) (models.Shipment, error) {
	v, err := s.repo.Shipment(ctx, id, false)
	if err != nil {
		return v, err
	}
	if err = s.repo.Active(ctx, u); err != nil {
		return models.Shipment{}, err
	}
	c, err := s.repo.Child(ctx, v.SellerOrderID)
	if err != nil {
		return models.Shipment{}, err
	}
	// Owner check reads only; no parent lock needed for a tracking view.
	var o models.Order
	if err = s.repo.orderView(ctx, c.OrderID, &o); err != nil {
		return models.Shipment{}, err
	}
	if o.UserID == u {
		return v, nil
	}
	if v.DriverID != nil && *v.DriverID == u {
		d, e := s.repo.Profile(ctx, u, false)
		if e == nil && d.Status == "approved" {
			return v, nil
		}
	}
	if err = s.repo.Seller(ctx, u, c.SellerID); err == nil {
		return v, nil
	}
	if err = s.repo.Admin(ctx, u); err == nil {
		return v, nil
	}
	return models.Shipment{}, shared.ErrForbidden
}
func (s *Service) Events(ctx context.Context, u, id uuid.UUID, p, l int) ([]models.ShipmentEvent, error) {
	if !page(p, l) {
		return nil, shared.ErrInvalid
	}
	if _, err := s.Detail(ctx, u, id); err != nil {
		return nil, err
	}
	return s.repo.Events(ctx, id, p, l)
}
