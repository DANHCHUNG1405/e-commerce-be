package shared

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrForbidden = errors.New("forbidden")
var ErrInvalid = errors.New("invalid input")
var ErrConflict = errors.New("conflict")
var ErrUnavailable = errors.New("service unavailable")
var ErrRateLimited = errors.New("rate limit exceeded")

// Repository is transaction-scoped. Callers supply fixed SQL, never client SQL.
type Repository struct{ DB *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{DB: db} }
func (r *Repository) Within(ctx context.Context, fn func(*Repository) error) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(New(tx)) })
}
func (r *Repository) Admin(ctx context.Context, user uuid.UUID) error {
	var n int64
	err := r.DB.WithContext(ctx).Table("user_roles ur").Joins("JOIN roles r ON r.id=ur.role_id").Joins("JOIN users u ON u.id=ur.user_id").Where("ur.user_id=? AND r.name='admin' AND u.deleted_at IS NULL AND r.deleted_at IS NULL", user).Count(&n).Error
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrForbidden
	}
	return nil
}
func (r *Repository) Seller(ctx context.Context, user, seller uuid.UUID) error {
	var n int64
	err := r.DB.WithContext(ctx).Table("seller_members sm").Joins("JOIN sellers s ON s.id=sm.seller_id").Joins("JOIN users u ON u.id=sm.user_id").Where("sm.user_id=? AND sm.seller_id=? AND sm.role IN ('owner','manager','staff') AND s.deleted_at IS NULL AND u.deleted_at IS NULL", user, seller).Count(&n).Error
	if err != nil {
		return err
	}
	if n == 0 {
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
