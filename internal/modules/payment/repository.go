package payment

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

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Admin(ctx context.Context, user uuid.UUID) error {
	return shared.New(r.db).Admin(ctx, user)
}
func (r *Repository) Receipts(ctx context.Context, status string, page, limit int) ([]models.PaymentWebhookReceipt, error) {
	v := []models.PaymentWebhookReceipt{}
	q := r.db.WithContext(ctx)
	if status != "" {
		q = q.Where("status=?", status)
	}
	err := q.Order("created_at DESC,id").Offset((page - 1) * limit).Limit(limit).Find(&v).Error
	return v, err
}

func (r *Repository) OwnPayment(ctx context.Context, user, order uuid.UUID) (models.Payment, error) {
	p := models.Payment{}
	err := r.db.WithContext(ctx).Where("order_id=? AND order_id IN (SELECT id FROM orders WHERE user_id=? AND deleted_at IS NULL)", order, user).First(&p).Error
	return p, err
}

func (r *Repository) Within(ctx context.Context, fn func(*Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(NewRepository(tx)) })
}
func (r *Repository) InsertReceipt(rec *models.PaymentWebhookReceipt) (bool, error) {
	result := r.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "provider"}, {Name: "provider_transaction_id"}}, DoNothing: true}).Create(rec)
	return result.RowsAffected > 0, result.Error
}
func (r *Repository) ExistingReceipt(provider, id string) (models.PaymentWebhookReceipt, error) {
	var rec models.PaymentWebhookReceipt
	err := r.db.Where("provider=? AND provider_transaction_id=?", provider, id).First(&rec).Error
	return rec, err
}
func (r *Repository) PaymentByCode(code string) (models.Payment, bool, error) {
	var p models.Payment
	result := r.db.Where("code=? AND method='sepay'", code).Limit(1).Find(&p)
	return p, result.RowsAffected > 0, result.Error
}
func (r *Repository) LockOrderPayment(p *models.Payment) (models.Order, error) {
	// Keep the same lock order as cancellation and fulfillment.
	var o models.Order
	if err := r.db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&o, "id=?", p.OrderID).Error; err != nil {
		return o, err
	}
	err := r.db.Clauses(clause.Locking{Strength: "UPDATE"}).First(p, "id=?", p.ID).Error
	return o, err
}
func (r *Repository) MarkPaid(p *models.Payment) error {
	return r.db.Model(p).Update("status", "paid").Error
}
func (r *Repository) RecordTransaction(v *models.PaymentTransaction) error {
	return r.db.Create(v).Error
}
func (r *Repository) SaveReceipt(rec *models.PaymentWebhookReceipt) error {
	return r.db.Model(rec).Updates(map[string]any{"status": rec.Status, "payment_id": rec.PaymentID}).Error
}
func (r *Repository) Enqueue(kind string, id uuid.UUID, event string, payload map[string]any) error {
	return outbox.Enqueue(r.db, kind, id, event, payload)
}
