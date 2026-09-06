package chat

import (
	"context"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

type Publisher interface {
	Publish([]uuid.UUID, string, any)
}
type Limiter interface {
	Allow(context.Context, uuid.UUID, string, int, time.Duration) error
}
type Service struct {
	repo      *Repository
	publisher Publisher
	limiter   Limiter
}

func New(r *Repository, p Publisher, l Limiter) *Service { return &Service{r, p, l} }
func (s *Service) Active(ctx context.Context, user uuid.UUID) error {
	return s.repo.ActiveUser(ctx, user)
}
func (s *Service) limit(ctx context.Context, user uuid.UUID, kind string, n int, d time.Duration) error {
	if s.limiter == nil {
		return shared.ErrUnavailable
	}
	return s.limiter.Allow(ctx, user, kind, n, d)
}
func (s *Service) Open(ctx context.Context, user, seller uuid.UUID) (models.ChatConversation, error) {
	if seller == uuid.Nil || user == uuid.Nil {
		return models.ChatConversation{}, shared.ErrInvalid
	}
	if err := s.limit(ctx, user, "open", 20, time.Minute); err != nil {
		return models.ChatConversation{}, err
	}
	return s.repo.Open(ctx, user, seller)
}
func (s *Service) List(ctx context.Context, user uuid.UUID, page, limit int) ([]Conversation, error) {
	if page < 1 || page > 100000 || limit < 1 || limit > 100 {
		return nil, shared.ErrInvalid
	}
	if err := s.Active(ctx, user); err != nil {
		return nil, err
	}
	return s.repo.List(ctx, user, page, limit)
}
func (s *Service) Messages(ctx context.Context, user, id uuid.UUID, after, before *int64, limit int) ([]models.ChatMessage, error) {
	if limit < 1 || limit > 100 || (after != nil && before != nil) || (after != nil && *after < 0) || (before != nil && *before <= 0) {
		return nil, shared.ErrInvalid
	}
	if _, err := s.repo.Access(ctx, user, id, false); err != nil {
		return nil, err
	}
	return s.repo.Messages(ctx, id, after, before, limit)
}

type SendInput struct {
	ConversationID  uuid.UUID `json:"conversationId"`
	ClientMessageID uuid.UUID `json:"clientMessageId"`
	Body            string    `json:"body"`
}

func (in SendInput) Validate() error {
	if in.ConversationID == uuid.Nil || in.ClientMessageID == uuid.Nil || !utf8.ValidString(in.Body) || strings.TrimSpace(in.Body) == "" || utf8.RuneCountInString(in.Body) > 5000 || len(in.Body) > 20000 {
		return shared.ErrInvalid
	}
	return nil
}
func (s *Service) Send(ctx context.Context, user uuid.UUID, in SendInput) (models.ChatMessage, error) {
	var m models.ChatMessage
	if err := in.Validate(); err != nil {
		return m, err
	}
	if err := s.limit(ctx, user, "send", 60, time.Minute); err != nil {
		return m, err
	}
	err := s.repo.Within(ctx, func(tx *Repository) error {
		c, err := tx.Access(ctx, user, in.ConversationID, true)
		if err != nil {
			return err
		}
		old, found, err := tx.Existing(ctx, in.ConversationID, user, in.ClientMessageID)
		if err != nil {
			return err
		}
		if found {
			if old.Body != in.Body {
				return shared.ErrConflict
			}
			m = old
			return nil
		}
		if c.LastSequence == math.MaxInt64 {
			return shared.ErrConflict
		}
		m = models.ChatMessage{ConversationID: in.ConversationID, SenderID: user, ClientMessageID: in.ClientMessageID, Body: in.Body, Sequence: c.LastSequence + 1}
		return tx.Insert(ctx, &m)
	})
	if err == nil {
		s.notify(ctx, in.ConversationID, "chat:message", m)
	}
	return m, err
}
func (s *Service) Read(ctx context.Context, user, id uuid.UUID, seq int64) (models.ChatRead, error) {
	v := models.ChatRead{ConversationID: id, UserID: user, LastSequence: seq, UpdatedAt: time.Now().UTC()}
	if id == uuid.Nil || seq < 0 {
		return v, shared.ErrInvalid
	}
	if err := s.limit(ctx, user, "read", 120, time.Minute); err != nil {
		return v, err
	}
	err := s.repo.Within(ctx, func(tx *Repository) error {
		c, err := tx.Access(ctx, user, id, true)
		if err != nil {
			return err
		}
		if seq > c.LastSequence {
			return shared.ErrInvalid
		}
		return tx.Read(ctx, &v)
	})
	if err == nil {
		s.notify(ctx, id, "chat:read", v)
	}
	return v, err
}
func (s *Service) Typing(ctx context.Context, user, id uuid.UUID, typing bool) error {
	if id == uuid.Nil {
		return shared.ErrInvalid
	}
	if err := s.limit(ctx, user, "typing", 20, 10*time.Second); err != nil {
		return err
	}
	if _, err := s.repo.Access(ctx, user, id, false); err != nil {
		return err
	}
	s.notify(ctx, id, "chat:typing", map[string]any{"conversationId": id, "userId": user, "typing": typing})
	return nil
}
func (s *Service) notify(ctx context.Context, id uuid.UUID, event string, payload any) {
	// Delivery is best-effort after commit; history + per-conversation sequence recovers gaps.
	if s.publisher == nil {
		return
	}
	recipients, err := s.repo.Recipients(ctx, id)
	if err == nil {
		s.publisher.Publish(recipients, event, payload)
	}
}
