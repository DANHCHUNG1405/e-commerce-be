package models

import (
	"github.com/google/uuid"
	"time"
)

type Base struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time `gorm:"index" json:"-"`
}
type User struct {
	Base
	Email    string `gorm:"uniqueIndex;not null" json:"email"`
	Password string `gorm:"not null" json:"-"`
	FullName string `json:"fullName"`
}
type Role struct {
	Base
	Name string `gorm:"uniqueIndex;not null" json:"name"`
}
type UserRole struct {
	UserID uuid.UUID `gorm:"type:uuid;primaryKey"`
	RoleID uuid.UUID `gorm:"type:uuid;primaryKey"`
}
type Seller struct {
	Base
	Name           string `gorm:"not null"`
	Slug           string `gorm:"uniqueIndex;not null"`
	CommissionRate int    `gorm:"not null;default:0"`
}
type SellerMember struct {
	SellerID uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID   uuid.UUID `gorm:"type:uuid;primaryKey"`
	Role     string    `gorm:"not null"`
}
type RefreshToken struct {
	Base
	UserID    uuid.UUID `gorm:"type:uuid;index;not null"`
	TokenHash string    `gorm:"uniqueIndex;not null"`
	ExpiresAt time.Time `gorm:"not null"`
	RevokedAt *time.Time
}
type Category struct {
	Base
	ParentID *uuid.UUID `gorm:"type:uuid;index"`
	Name     string     `gorm:"not null"`
	Slug     string     `gorm:"uniqueIndex;not null"`
}
type Product struct {
	Base
	SellerID    uuid.UUID `gorm:"type:uuid;index;not null"`
	Name        string    `gorm:"not null"`
	Slug        string    `gorm:"uniqueIndex;not null"`
	Description string    `gorm:"type:text"`
	Status      string    `gorm:"not null;default:draft"`
	MetadataID  *string
}
type ProductVariant struct {
	Base
	ProductID  uuid.UUID      `gorm:"type:uuid;index;not null"`
	SKU        string         `gorm:"uniqueIndex;not null"`
	Name       string         `gorm:"not null"`
	Price      int64          `gorm:"not null"`
	Stock      int            `gorm:"not null;default:0"`
	Attributes map[string]any `gorm:"serializer:json"`
}
type ProductImage struct {
	Base
	ProductID uuid.UUID `gorm:"type:uuid;index;not null"`
	URL       string    `gorm:"not null"`
	SortOrder int       `gorm:"not null;default:0"`
}
type ProductCategory struct {
	ProductID  uuid.UUID `gorm:"type:uuid;primaryKey"`
	CategoryID uuid.UUID `gorm:"type:uuid;primaryKey"`
}
type InventoryMovement struct {
	Base
	VariantID   uuid.UUID `gorm:"type:uuid;index;not null"`
	Quantity    int       `gorm:"not null"`
	Type        string    `gorm:"not null"`
	ReferenceID *uuid.UUID
	Note        string
}
type Cart struct {
	Base
	UserID uuid.UUID `gorm:"type:uuid;uniqueIndex;not null"`
}
type CartItem struct {
	Base
	CartID    uuid.UUID `gorm:"type:uuid;not null;index"`
	VariantID uuid.UUID `gorm:"type:uuid;not null;index"`
	Quantity  int       `gorm:"not null"`
}
type Order struct {
	Base
	UserID                                 uuid.UUID `gorm:"type:uuid;index;not null"`
	Status                                 string    `gorm:"not null;default:pending"`
	Currency                               string    `gorm:"not null;default:VND"`
	Subtotal, Discount, ShippingFee, Total int64     `gorm:"not null;default:0"`
	ShippingAddressID                      uuid.UUID `gorm:"type:uuid;not null"`
}
type SellerOrder struct {
	Base
	OrderID, SellerID           uuid.UUID `gorm:"type:uuid;index;not null"`
	Status                      string    `gorm:"not null;default:pending"`
	Subtotal, Commission, Total int64     `gorm:"not null;default:0"`
}
type OrderItem struct {
	Base
	SellerOrderID, VariantID      uuid.UUID `gorm:"type:uuid;index;not null"`
	ProductName, SKU, VariantName string    `gorm:"not null"`
	UnitPrice                     int64     `gorm:"not null"`
	Quantity                      int       `gorm:"not null"`
	Discount                      int64     `gorm:"not null;default:0"`
}
type ShippingAddress struct {
	Base
	UserID                                                               uuid.UUID `gorm:"type:uuid;index;not null"`
	RecipientName, Phone, AddressLine, Ward, District, Province, Country string    `gorm:"not null"`
	PostalCode                                                           string
}
type Payment struct {
	Base
	OrderID uuid.UUID `gorm:"type:uuid;uniqueIndex;not null"`
	Method  string    `gorm:"not null"`
	Status  string    `gorm:"not null;default:pending"`
	Amount  int64     `gorm:"not null"`
}
type PaymentTransaction struct {
	Base
	PaymentID                               uuid.UUID `gorm:"type:uuid;index;not null"`
	Provider, ProviderTransactionID, Status string    `gorm:"not null"`
	Amount                                  int64     `gorm:"not null"`
}
type Shipment struct {
	Base
	SellerOrderID           uuid.UUID `gorm:"type:uuid;uniqueIndex;not null"`
	Carrier, TrackingNumber string
	Status                  string `gorm:"not null;default:pending"`
}
type ShipmentEvent struct {
	Base
	ShipmentID  uuid.UUID `gorm:"type:uuid;index;not null"`
	Status      string    `gorm:"not null"`
	Description string
}
type Coupon struct {
	Base
	Code                  string `gorm:"uniqueIndex;not null"`
	Type                  string `gorm:"not null"`
	Value                 int64  `gorm:"not null"`
	StartsAt, EndsAt      time.Time
	UsageLimit, UsedCount int
}
type CouponRule struct {
	Base
	CouponID uuid.UUID `gorm:"type:uuid;index;not null"`
	MinOrder int64
	SellerID *uuid.UUID `gorm:"type:uuid"`
}
type CouponRedemption struct {
	Base
	CouponID, UserID, OrderID uuid.UUID `gorm:"type:uuid;index;not null"`
}
type Review struct {
	Base
	UserID, ProductID uuid.UUID  `gorm:"type:uuid;index;not null"`
	OrderItemID       *uuid.UUID `gorm:"type:uuid"`
	Rating            int        `gorm:"not null"`
	Comment           string
	Status            string `gorm:"not null;default:published"`
}
type Wishlist struct {
	Base
	UserID uuid.UUID `gorm:"type:uuid;uniqueIndex;not null"`
}
type WishlistItem struct {
	Base
	WishlistID, ProductID uuid.UUID `gorm:"type:uuid;index;not null"`
}
type OutboxEvent struct {
	Base
	AggregateType string    `gorm:"not null;index"`
	AggregateID   uuid.UUID `gorm:"type:uuid;not null;index"`
	EventType     string    `gorm:"not null;index"`
	Payload       []byte    `gorm:"type:jsonb;not null"`
	Status        string    `gorm:"not null;default:pending;index"`
	RetryCount    int       `gorm:"not null;default:0"`
	AvailableAt   time.Time
	ProcessedAt   *time.Time
}
