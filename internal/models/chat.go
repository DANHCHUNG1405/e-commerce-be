package models

import (
	"github.com/google/uuid"
	"time"
)

type ChatConversation struct {
	Base
	BuyerID      uuid.UUID `json:"buyerId"`
	SellerID     uuid.UUID `json:"sellerId"`
	LastSequence int64     `json:"lastSequence"`
}
type ChatConversationView struct {
	ChatConversation
	UnreadCount      int64 `json:"unreadCount"`
	LastReadSequence int64 `json:"lastReadSequence"`
}
type ChatMessage struct {
	Base
	ConversationID  uuid.UUID `json:"conversationId"`
	SenderID        uuid.UUID `json:"senderId"`
	ClientMessageID uuid.UUID `json:"clientMessageId"`
	Sequence        int64     `json:"sequence"`
	Body            string    `json:"body"`
}
type ChatRead struct {
	ConversationID uuid.UUID `gorm:"primaryKey" json:"conversationId"`
	UserID         uuid.UUID `gorm:"primaryKey" json:"userId"`
	LastSequence   int64     `json:"lastSequence"`
	UpdatedAt      time.Time `json:"updatedAt"`
}
