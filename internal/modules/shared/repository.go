package shared

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrForbidden = errors.New("forbidden")
var ErrUnauthorized = errors.New("unauthorized")
var ErrInvalid = errors.New("invalid input")
var ErrConflict = errors.New("conflict")
var ErrUnavailable = errors.New("service unavailable")
var ErrRateLimited = errors.New("rate limit exceeded")

// Repository is transaction-scoped. Callers supply fixed SQL, never client SQL.
type Repository struct{ DB *gorm.DB }

type sellerDirectoryKey struct{}
type SellerDirectory interface {
	CheckSellerMembership(context.Context, uuid.UUID, uuid.UUID) (string, string, error)
	GetSellerInfo(context.Context, uuid.UUID) (SellerInfo, error)
	BatchSellerInfo(context.Context, []uuid.UUID) (map[uuid.UUID]SellerInfo, error)
	GetSellerOperations(context.Context, uuid.UUID) (SellerOperations, error)
	CountSellers(context.Context, string) (int64, error)
}

type SellerInfo struct {
	Status         string
	CommissionRate int32
}
type SellerOperations struct {
	SellerInfo
	PickupAddress map[string]any
}

func sellerDirectory(ctx context.Context) (SellerDirectory, error) {
	directory, ok := ctx.Value(sellerDirectoryKey{}).(SellerDirectory)
	if !ok || directory == nil {
		return nil, ErrUnavailable
	}
	return directory, nil
}

func GetSellerInfo(ctx context.Context, seller uuid.UUID) (SellerInfo, error) {
	directory, err := sellerDirectory(ctx)
	if err != nil {
		return SellerInfo{}, err
	}
	return directory.GetSellerInfo(ctx, seller)
}

func BatchSellerInfo(ctx context.Context, sellers []uuid.UUID) (map[uuid.UUID]SellerInfo, error) {
	directory, err := sellerDirectory(ctx)
	if err != nil {
		return nil, err
	}
	return directory.BatchSellerInfo(ctx, sellers)
}

func GetSellerOperations(ctx context.Context, seller uuid.UUID) (SellerOperations, error) {
	directory, err := sellerDirectory(ctx)
	if err != nil {
		return SellerOperations{}, err
	}
	return directory.GetSellerOperations(ctx, seller)
}

func SellerMembership(ctx context.Context, user, seller uuid.UUID) (string, string, error) {
	directory, err := sellerDirectory(ctx)
	if err != nil {
		return "", "", err
	}
	return directory.CheckSellerMembership(ctx, user, seller)
}

func CountSellers(ctx context.Context, status string) (int64, error) {
	directory, err := sellerDirectory(ctx)
	if err != nil {
		return 0, err
	}
	return directory.CountSellers(ctx, status)
}

func WithSellerDirectory(ctx context.Context, directory SellerDirectory) context.Context {
	return context.WithValue(ctx, sellerDirectoryKey{}, directory)
}

func New(db *gorm.DB) *Repository { return &Repository{DB: db} }
func (r *Repository) Within(ctx context.Context, fn func(*Repository) error) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(New(tx)) })
}
func (r *Repository) Admin(ctx context.Context, user uuid.UUID) error {
	var n int64
	err := r.DB.WithContext(ctx).Table("identity.user_roles ur").Joins("JOIN identity.roles r ON r.id=ur.role_id").Joins("JOIN identity.users u ON u.id=ur.user_id").Where("ur.user_id=? AND r.name='admin' AND u.deleted_at IS NULL AND r.deleted_at IS NULL", user).Count(&n).Error
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrForbidden
	}
	return nil
}
func (r *Repository) Seller(ctx context.Context, user, seller uuid.UUID) error {
	directory, err := sellerDirectory(ctx)
	if err != nil {
		return err
	}
	role, _, err := directory.CheckSellerMembership(ctx, user, seller)
	if err != nil {
		return err
	}
	if role != "owner" && role != "manager" && role != "staff" {
		return ErrForbidden
	}
	return nil
}
func (r *Repository) One(ctx context.Context, out any, where string, args ...any) error {
	return r.DB.WithContext(ctx).Where(where, args...).First(out).Error
}
func (r *Repository) List(ctx context.Context, out any, where string, args []any, page, limit int) error {
	return r.DB.WithContext(ctx).Where(where, args...).Order("created_at DESC, id").Limit(limit).Offset((page - 1) * limit).Find(out).Error
}
func (r *Repository) Create(ctx context.Context, value any) error {
	return r.DB.WithContext(ctx).Create(value).Error
}
func (r *Repository) Update(ctx context.Context, model any, where string, args []any, fields map[string]any) error {
	result := r.DB.WithContext(ctx).Model(model).Where(where, args...).Updates(fields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
