package chat

import (
	"context"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
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
func (r *Repository) ActiveUser(ctx context.Context, user uuid.UUID) error {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.User{}).Where("id=? AND deleted_at IS NULL", user).Count(&n).Error
	if err != nil {
		return err
	}
	if n == 0 {
		return shared.ErrForbidden
	}
	return nil
}
func (r *Repository) Access(ctx context.Context, user, id uuid.UUID, lock bool) (models.ChatConversation, error) {
	c := models.ChatConversation{}
	q := r.db.WithContext(ctx)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.Where("id=? AND deleted_at IS NULL AND EXISTS (SELECT 1 FROM users WHERE id=? AND deleted_at IS NULL) AND (buyer_id=? OR seller_id IN (SELECT sm.seller_id FROM seller_members sm JOIN sellers s ON s.id=sm.seller_id WHERE sm.user_id=? AND sm.role IN ('owner','manager','staff') AND s.deleted_at IS NULL))", id, user, user, user).First(&c).Error
	return c, err
}
func (r *Repository) Open(ctx context.Context, buyer, seller uuid.UUID) (models.ChatConversation, error) {
	c := models.ChatConversation{BuyerID: buyer, SellerID: seller}
	if err := r.ActiveUser(ctx, buyer); err != nil {
		return c, err
	}
	var s models.Seller
	if err := r.db.WithContext(ctx).Where("id=? AND status='approved' AND deleted_at IS NULL", seller).First(&s).Error; err != nil {
		return c, err
	}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "buyer_id"}, {Name: "seller_id"}}, DoNothing: true}).Create(&c).Error; err != nil {
		return c, err
	}
	err := r.db.WithContext(ctx).Where("buyer_id=? AND seller_id=? AND deleted_at IS NULL", buyer, seller).First(&c).Error
	return c, err
}

type Conversation struct {
	models.ChatConversation
	UnreadCount      int64 `json:"unreadCount"`
	LastReadSequence int64 `json:"lastReadSequence"`
}

func (r *Repository) List(ctx context.Context, user uuid.UUID, page, limit int) ([]Conversation, error) {
	v := []Conversation{}
	err := r.db.WithContext(ctx).Table("chat_conversations c").Select("c.*, COALESCE(cr.last_sequence,0) AS last_read_sequence, (SELECT COUNT(*) FROM chat_messages m WHERE m.conversation_id=c.id AND m.sender_id<>? AND m.sequence>COALESCE(cr.last_sequence,0)) AS unread_count", user).Joins("LEFT JOIN chat_reads cr ON cr.conversation_id=c.id AND cr.user_id=?", user).Where("c.deleted_at IS NULL AND (c.buyer_id=? OR c.seller_id IN (SELECT sm.seller_id FROM seller_members sm JOIN sellers s ON s.id=sm.seller_id WHERE sm.user_id=? AND sm.role IN ('owner','manager','staff') AND s.deleted_at IS NULL))", user, user).Order("c.updated_at DESC,c.id").Offset((page - 1) * limit).Limit(limit).Scan(&v).Error
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
func (r *Repository) Recipients(ctx context.Context, id uuid.UUID) ([]uuid.UUID, error) {
	v := []uuid.UUID{}
	err := r.db.WithContext(ctx).Raw("SELECT u.id FROM users u WHERE u.deleted_at IS NULL AND (u.id IN (SELECT buyer_id FROM chat_conversations WHERE id=?) OR u.id IN (SELECT sm.user_id FROM seller_members sm JOIN chat_conversations c ON c.seller_id=sm.seller_id JOIN sellers s ON s.id=sm.seller_id WHERE c.id=? AND sm.role IN ('owner','manager','staff') AND s.deleted_at IS NULL))", id, id).Scan(&v).Error
	return v, err
}
