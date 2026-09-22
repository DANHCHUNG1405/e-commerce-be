package chat

import (
	"context"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Within(ctx context.Context, fn func(*Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(NewRepository(tx)) })
}
func (r *Repository) Access(ctx context.Context, id uuid.UUID, lock bool) (models.ChatConversation, error) {
	c := models.ChatConversation{}
	q := r.db.WithContext(ctx)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.Where("id=? AND deleted_at IS NULL", id).First(&c).Error
	return c, err
}
func (r *Repository) Open(ctx context.Context, buyer, seller uuid.UUID) (models.ChatConversation, error) {
	c := models.ChatConversation{BuyerID: buyer, SellerID: seller}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "buyer_id"}, {Name: "seller_id"}}, DoNothing: true}).Create(&c).Error; err != nil {
		return c, err
	}
	err := r.db.WithContext(ctx).Where("buyer_id=? AND seller_id=? AND deleted_at IS NULL", buyer, seller).First(&c).Error
	return c, err
}

func (r *Repository) List(ctx context.Context, user uuid.UUID, sellerIDs []uuid.UUID, page, limit int) ([]models.ChatConversationView, error) {
	v := []models.ChatConversationView{}
	q := r.db.WithContext(ctx).Table("chat.chat_conversations c").Select("c.*, COALESCE(cr.last_sequence,0) AS last_read_sequence, (SELECT COUNT(*) FROM chat.chat_messages m WHERE m.conversation_id=c.id AND m.sender_id<>? AND m.sequence>COALESCE(cr.last_sequence,0)) AS unread_count", user).Joins("LEFT JOIN chat.chat_reads cr ON cr.conversation_id=c.id AND cr.user_id=?", user).Where("c.deleted_at IS NULL")
	if len(sellerIDs) == 0 {
		q = q.Where("c.buyer_id=?", user)
	} else {
		q = q.Where("c.buyer_id=? OR c.seller_id IN ?", user, sellerIDs)
	}
	err := q.Order("c.updated_at DESC,c.id").Offset((page - 1) * limit).Limit(limit).Scan(&v).Error
	return v, err
}
func (r *Repository) Messages(ctx context.Context, id uuid.UUID, after, before *int64, limit int) ([]models.ChatMessage, error) {
	v := []models.ChatMessage{}
	q := r.db.WithContext(ctx).Where("conversation_id=?", id)
	if after != nil {
		q = q.Where("sequence>?", *after).Order("sequence ASC")
	} else {
		if before != nil {
			q = q.Where("sequence<?", *before)
		}
		q = q.Order("sequence DESC")
	}
	if err := q.Limit(limit).Find(&v).Error; err != nil {
		return nil, err
	}
	if after == nil {
		for i, j := 0, len(v)-1; i < j; i, j = i+1, j-1 {
			v[i], v[j] = v[j], v[i]
		}
	}
	return v, nil
}
func (r *Repository) Existing(ctx context.Context, conversation, user, client uuid.UUID) (models.ChatMessage, bool, error) {
	var m models.ChatMessage
	result := r.db.WithContext(ctx).Where("conversation_id=? AND sender_id=? AND client_message_id=?", conversation, user, client).Limit(1).Find(&m)
	return m, result.RowsAffected > 0, result.Error
}
func (r *Repository) Insert(ctx context.Context, m *models.ChatMessage) error {
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(&models.ChatConversation{}).Where("id=?", m.ConversationID).Updates(map[string]any{"last_sequence": m.Sequence, "updated_at": time.Now().UTC()}).Error
}
func (r *Repository) Read(ctx context.Context, v *models.ChatRead) error {
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "conversation_id"}, {Name: "user_id"}}, DoUpdates: clause.Assignments(map[string]any{"last_sequence": gorm.Expr("GREATEST(chat_reads.last_sequence,EXCLUDED.last_sequence)"), "updated_at": v.UpdatedAt})}).Create(v).Error; err != nil {
		return err
	}
	return r.db.WithContext(ctx).Where("conversation_id=? AND user_id=?", v.ConversationID, v.UserID).First(v).Error
}
func (r *Repository) RecipientBuyer(ctx context.Context, id uuid.UUID) (uuid.UUID, uuid.UUID, error) {
	var row struct {
		BuyerID  *uuid.UUID
		SellerID uuid.UUID
	}
	err := r.db.WithContext(ctx).Table("chat.chat_conversations c").Select("u.id AS buyer_id, c.seller_id").Joins("LEFT JOIN identity.users u ON u.id=c.buyer_id AND u.deleted_at IS NULL").Where("c.id=? AND c.deleted_at IS NULL", id).Take(&row).Error
	if row.BuyerID == nil {
		return uuid.Nil, row.SellerID, err
	}
	return *row.BuyerID, row.SellerID, err
}
