package seller

import (
	"context"
	"encoding/json"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/example/e-commerce-be/internal/outbox"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"strings"
)

type Management struct{ repo *ManagementRepository }

func (s *Management) Profile(ctx context.Context, u, id uuid.UUID) (models.Seller, error) {
	if _, err := s.repo.Role(ctx, u, id, false); err != nil {
		return models.Seller{}, err
	}
	return s.repo.Profile(ctx, id)
}

func NewManagement(r *ManagementRepository) *Management { return &Management{r} }

type PickupAddress struct {
	RecipientName string `json:"recipientName"`
	Phone         string `json:"phone"`
	AddressLine   string `json:"addressLine"`
	Ward          string `json:"ward"`
	District      string `json:"district"`
	Province      string `json:"province"`
}
type ShopInput struct {
	Name          string        `json:"name"`
	Description   string        `json:"description"`
	PickupAddress PickupAddress `json:"pickupAddress"`
}

func (in ShopInput) pickupJSON() string { b, _ := json.Marshal(in.PickupAddress); return string(b) }
func (in ShopInput) Validate() error {
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 200 || len(in.Description) > 5000 {
		return shared.ErrInvalid
	}
	for _, v := range []string{in.PickupAddress.RecipientName, in.PickupAddress.Phone, in.PickupAddress.AddressLine, in.PickupAddress.Ward, in.PickupAddress.District, in.PickupAddress.Province} {
		if strings.TrimSpace(v) == "" || len(v) > 300 {
			return shared.ErrInvalid
		}
	}
	return nil
}
func enqueue(db *gorm.DB, id uuid.UUID, kind string, data map[string]any) error {
	return outbox.Enqueue(db, "seller", id, kind, data)
}
func pages(p, l int) bool { return p > 0 && p <= 100000 && l > 0 && l <= 100 }
func (s *Management) Update(ctx context.Context, u, id uuid.UUID, in ShopInput) error {
	if err := in.Validate(); err != nil {
		return err
	}
	return s.repo.Within(ctx, func(r *ManagementRepository) error {
		role, err := r.Role(ctx, u, id, true)
		if err != nil {
			return err
		}
		if role != "owner" && role != "manager" {
			return shared.ErrForbidden
		}
		if err := r.Update(ctx, id, in); err != nil {
			return err
		}
		return r.Audit(ctx, u, id, "seller_updated", map[string]any{})
	})
}
func (s *Management) Members(ctx context.Context, u, id uuid.UUID, p, l int) ([]models.SellerMember, error) {
	if !pages(p, l) {
		return nil, shared.ErrInvalid
	}
	if _, err := s.repo.Role(ctx, u, id, false); err != nil {
		return nil, err
	}
	return s.repo.Members(ctx, id, p, l)
}
func (s *Management) SetMember(ctx context.Context, u, id, target uuid.UUID, role string) error {
	if target == uuid.Nil || (role != "" && role != "manager" && role != "staff") {
		return shared.ErrInvalid
	}
	return s.repo.Within(ctx, func(r *ManagementRepository) error {
		own, err := r.Role(ctx, u, id, true)
		if err != nil {
			return err
		}
		if own != "owner" {
			return shared.ErrForbidden
		}
		old, found, err := r.Member(ctx, id, target)
		if err != nil {
			return err
		}
		if found && old.Role == "owner" {
			return shared.ErrConflict
		}
		if err := r.SetMember(ctx, id, target, role); err != nil {
			return err
		}
		return r.Audit(ctx, u, id, "seller_member_updated", map[string]any{"userId": target, "role": role})
	})
}
func (s *Management) Detail(ctx context.Context, u, shop, id uuid.UUID) (OrderDetail, error) {
	if _, err := s.repo.Role(ctx, u, shop, false); err != nil {
		return OrderDetail{}, err
	}
	return s.repo.Detail(ctx, shop, id)
}
func (s *Management) Orders(ctx context.Context, u, shop uuid.UUID, status string, p, l int) ([]models.SellerOrder, error) {
	if !pages(p, l) {
		return nil, shared.ErrInvalid
	}
	switch status {
	case "", "pending", "confirmed", "shipping", "delivered", "cancelled":
	default:
		return nil, shared.ErrInvalid
	}
	if _, err := s.repo.Role(ctx, u, shop, false); err != nil {
		return nil, err
	}
	return s.repo.Orders(ctx, shop, status, p, l)
}
func (s *Management) Dashboard(ctx context.Context, u, shop uuid.UUID) (Dashboard, error) {
	role, err := s.repo.Role(ctx, u, shop, false)
	if err != nil {
		return Dashboard{}, err
	}
	if role != "owner" && role != "manager" {
		return Dashboard{}, shared.ErrForbidden
	}
	return s.repo.Dashboard(ctx, shop)
}
func (s *Management) Inventory(ctx context.Context, u, shop, variant uuid.UUID, p, l int) ([]models.InventoryMovement, error) {
	if !pages(p, l) {
		return nil, shared.ErrInvalid
	}
	if _, err := s.repo.Role(ctx, u, shop, false); err != nil {
		return nil, err
	}
	return s.repo.Inventory(ctx, shop, variant, p, l)
}
