package assistant

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrInvalid     = errors.New("invalid assistant input")
	ErrForbidden   = errors.New("assistant conversation forbidden")
	ErrConflict    = errors.New("assistant conversation changed")
	ErrRateLimited = errors.New("assistant rate limit exceeded")
	ErrUnavailable = errors.New("assistant dependency unavailable")
)

type Conversation struct {
	ID           uuid.UUID `json:"id"`
	Title        string    `json:"title"`
	LastSequence int64     `json:"lastSequence"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
type Message struct {
	ID             uuid.UUID `json:"id"`
	ConversationID uuid.UUID `json:"conversationId"`
	Sequence       int64     `json:"sequence"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"createdAt"`
}

type ProductVariantContext struct {
	Name       string         `json:"name"`
	PriceVND   int64          `json:"priceVnd"`
	Stock      int            `json:"stock"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

type ProductContext struct {
	ID             uuid.UUID               `json:"id"`
	SellerID       uuid.UUID               `json:"sellerId"`
	Name           string                  `json:"name"`
	Description    string                  `json:"description,omitempty"`
	Variants       []ProductVariantContext `json:"variants,omitempty"`
	Specifications map[string]any          `json:"specifications,omitempty"`
}

type SuggestedAction struct {
	Type             string    `json:"type"`
	Label            string    `json:"label"`
	SellerID         uuid.UUID `json:"sellerId"`
	SuggestedMessage string    `json:"suggestedMessage"`
}

type Answer struct {
	Text            string
	SuggestedAction *SuggestedAction
}

var ErrProductUnavailable = errors.New("product unavailable to assistant")

type record struct {
	ID             uuid.UUID `gorm:"column:id"`
	ConversationID uuid.UUID `gorm:"column:conversation_id"`
	Sequence       int64     `gorm:"column:sequence"`
	Role           string    `gorm:"column:role"`
	Content        string    `gorm:"column:content"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}
type conversationRow struct {
	ID           uuid.UUID `gorm:"column:id"`
	UserID       uuid.UUID `gorm:"column:user_id"`
	Title        string    `gorm:"column:title"`
	LastSequence int64     `gorm:"column:last_sequence"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}
type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

type Model interface {
	Answer(context.Context, uuid.UUID, string, *uuid.UUID) (Answer, error)
}
type Service struct {
	repo    *Repository
	model   Model
	limiter Limiter
}

func NewService(repo *Repository, model Model, limiter Limiter) *Service {
	return &Service{repo: repo, model: model, limiter: limiter}
}

func (s *Service) Create(ctx context.Context, user uuid.UUID) (Conversation, error) {
	if user == uuid.Nil {
		return Conversation{}, ErrInvalid
	}
	c := conversationRow{ID: uuid.New(), UserID: user}
	if err := s.repo.db.WithContext(ctx).Table("assistant.conversations").Create(&c).Error; err != nil {
		return Conversation{}, err
	}
	return conversation(c), nil
}
func (s *Service) List(ctx context.Context, user uuid.UUID) ([]Conversation, error) {
	var rows []conversationRow
	err := s.repo.db.WithContext(ctx).Table("assistant.conversations").Where("user_id=? AND deleted_at IS NULL", user).Order("updated_at DESC").Limit(50).Scan(&rows).Error
	result := make([]Conversation, 0, len(rows))
	for _, row := range rows {
		result = append(result, conversation(row))
	}
	return result, err
}
func (s *Service) Messages(ctx context.Context, user, id uuid.UUID) ([]Message, error) {
	var c conversationRow
	if err := s.repo.db.WithContext(ctx).Table("assistant.conversations").Where("id=? AND user_id=? AND deleted_at IS NULL", id, user).Take(&c).Error; err != nil {
		return nil, ErrForbidden
	}
	var rows []record
	err := s.repo.db.WithContext(ctx).Table("assistant.messages").Where("conversation_id=?", id).Order("sequence DESC").Limit(100).Scan(&rows).Error
	result := make([]Message, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		result = append(result, message(rows[i]))
	}
	return result, err
}
func (s *Service) Send(ctx context.Context, user, id uuid.UUID, text string, productID *uuid.UUID) (Message, Message, *SuggestedAction, error) {
	text = strings.TrimSpace(text)
	if user == uuid.Nil || id == uuid.Nil || (productID != nil && *productID == uuid.Nil) || text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > 4000 {
		return Message{}, Message{}, nil, ErrInvalid
	}
	if s.limiter == nil {
		return Message{}, Message{}, nil, ErrUnavailable
	}
	if err := s.limiter.Allow(ctx, user, 15, time.Minute); err != nil {
		return Message{}, Message{}, nil, err
	}
	var c conversationRow
	if err := s.repo.db.WithContext(ctx).Table("assistant.conversations").Where("id=? AND user_id=? AND deleted_at IS NULL", id, user).Take(&c).Error; err != nil {
		return Message{}, Message{}, nil, ErrForbidden
	}
	answer, err := s.model.Answer(ctx, user, text, productID)
	if err != nil {
		return Message{}, Message{}, nil, err
	}
	answer.Text = strings.TrimSpace(answer.Text)
	if answer.Text == "" || utf8.RuneCountInString(answer.Text) > 12000 {
		return Message{}, Message{}, nil, errors.New("assistant returned an invalid response")
	}
	userMessage := record{ID: uuid.New(), ConversationID: id, Sequence: c.LastSequence + 1, Role: "user", Content: text}
	assistantMessage := record{ID: uuid.New(), ConversationID: id, Sequence: c.LastSequence + 2, Role: "assistant", Content: answer.Text}
	err = s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updated := tx.Table("assistant.conversations").Where("id=? AND user_id=? AND last_sequence=? AND deleted_at IS NULL", id, user, c.LastSequence).Updates(map[string]any{"last_sequence": c.LastSequence + 2, "updated_at": time.Now().UTC(), "title": titleFor(c.Title, text)})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrConflict
		}
		if err := tx.Table("assistant.messages").Create(&userMessage).Error; err != nil {
			return err
		}
		return tx.Table("assistant.messages").Create(&assistantMessage).Error
	})
	return message(userMessage), message(assistantMessage), answer.SuggestedAction, err
}
func titleFor(old, text string) string {
	if old != "" {
		return old
	}
	r := []rune(text)
	if len(r) > 80 {
		r = r[:80]
	}
	return string(r)
}
func conversation(r conversationRow) Conversation {
	return Conversation{ID: r.ID, Title: r.Title, LastSequence: r.LastSequence, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
}
func message(r record) Message {
	return Message{ID: r.ID, ConversationID: r.ConversationID, Sequence: r.Sequence, Role: r.Role, Content: r.Content, CreatedAt: r.CreatedAt.UTC()}
}
