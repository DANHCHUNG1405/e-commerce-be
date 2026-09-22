package user

import (
	"context"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
	"strings"
	"time"
)

type Service struct{ repo *shared.Repository }

type Permissions struct {
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
}

type SellerMembership struct {
	SellerID     uuid.UUID `json:"sellerId"`
	SellerName   string    `json:"sellerName"`
	SellerSlug   string    `json:"sellerSlug"`
	SellerStatus string    `json:"sellerStatus"`
	Role         string    `json:"role"`
}

func New(r *shared.Repository) *Service { return &Service{repo: r} }

func (s *Service) Permissions(ctx context.Context, u uuid.UUID) (Permissions, error) {
	var roles []string
	err := s.repo.DB.WithContext(ctx).Table("identity.roles r").Select("r.name").Joins("JOIN identity.user_roles ur ON ur.role_id=r.id").Where("ur.user_id=?", u).Order("r.name").Scan(&roles).Error
	if err != nil {
		return Permissions{}, err
	}
	permissions := make([]string, 0, len(roles))
	for _, role := range roles {
		switch role {
		case "admin":
			permissions = append(permissions, "admin:*")
		case "customer":
			permissions = append(permissions, "customer")
		case "driver":
			permissions = append(permissions, "driver")
		default:
			permissions = append(permissions, role)
		}
	}
	return Permissions{Roles: roles, Permissions: permissions}, nil
}

func (s *Service) SellerMemberships(ctx context.Context, u uuid.UUID) ([]SellerMembership, error) {
	items := []SellerMembership{}
	err := s.repo.DB.WithContext(ctx).Table("seller_members sm").Select("sm.seller_id, s.name AS seller_name, s.slug AS seller_slug, s.status AS seller_status, sm.role").Joins("JOIN sellers s ON s.id=sm.seller_id").Where("sm.user_id=? AND s.deleted_at IS NULL", u).Order("s.name, sm.role").Scan(&items).Error
	return items, err
}
func (s *Service) Profile(ctx context.Context, user uuid.UUID, name string) error {
	if strings.TrimSpace(name) == "" || len(name) > 200 {
		return shared.ErrInvalid
	}
	return s.repo.Update(ctx, &models.User{}, "id=? AND deleted_at IS NULL", []any{user}, map[string]any{"full_name": name})
}

type AddressInput struct {
	RecipientName string `json:"recipientName" binding:"required,max=200"`
	Phone         string `json:"phone" binding:"required,max=30"`
	AddressLine   string `json:"addressLine" binding:"required,max=500"`
	Ward          string `json:"ward" binding:"required,max=100"`
	District      string `json:"district" binding:"required,max=100"`
	Province      string `json:"province" binding:"required,max=100"`
	Country       string `json:"country" binding:"required,len=2"`
	PostalCode    string `json:"postalCode" binding:"max=20"`
}

func (s *Service) Addresses(ctx context.Context, user uuid.UUID, p, l int) ([]models.ShippingAddress, error) {
	v := []models.ShippingAddress{}
	err := s.repo.List(ctx, &v, "user_id=? AND deleted_at IS NULL", []any{user}, p, l)
	return v, err
}
func (s *Service) SaveAddress(ctx context.Context, user, id uuid.UUID, in AddressInput) (models.ShippingAddress, error) {
	v := models.ShippingAddress{UserID: user, RecipientName: in.RecipientName, Phone: in.Phone, AddressLine: in.AddressLine, Ward: in.Ward, District: in.District, Province: in.Province, Country: in.Country, PostalCode: in.PostalCode}
	if in.RecipientName == "" || in.Phone == "" || in.AddressLine == "" || in.Province == "" || len(in.Country) != 2 {
		return v, shared.ErrInvalid
	}
	if id == uuid.Nil {
		err := s.repo.Create(ctx, &v)
		return v, err
	}
	v.ID = id
	err := s.repo.Update(ctx, &models.ShippingAddress{}, "id=? AND user_id=? AND deleted_at IS NULL", []any{id, user}, map[string]any{"recipient_name": in.RecipientName, "phone": in.Phone, "address_line": in.AddressLine, "ward": in.Ward, "district": in.District, "province": in.Province, "country": in.Country, "postal_code": in.PostalCode})
	return v, err
}
func (s *Service) DeleteAddress(ctx context.Context, user, id uuid.UUID) error {
	return s.repo.Update(ctx, &models.ShippingAddress{}, "id=? AND user_id=? AND deleted_at IS NULL", []any{id, user}, map[string]any{"deleted_at": time.Now().UTC()})
}
