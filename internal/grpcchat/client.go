package grpcchat

import (
	"context"
	"time"

	chatv1 "github.com/example/e-commerce-be/internal/gen/chat/v1"
	"github.com/example/e-commerce-be/internal/grpcutil"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Client struct {
	rpc chatv1.ChatServiceClient
}

func Dial(target string) (*grpc.ClientConn, *Client, error) {
	connection, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	return connection, &Client{rpc: chatv1.NewChatServiceClient(connection)}, nil
}

func (c *Client) Open(ctx context.Context, authorization string, sellerID uuid.UUID) (models.ChatConversation, error) {
	ctx, cancel := requestContext(ctx, authorization)
	defer cancel()
	response, err := c.rpc.OpenConversation(ctx, &chatv1.OpenConversationRequest{SellerId: sellerID.String()})
	if err != nil {
		return models.ChatConversation{}, grpcutil.FromStatus(err)
	}
	return conversationModel(response), nil
}

func (c *Client) List(ctx context.Context, authorization string, page, limit int) ([]models.ChatConversationView, error) {
	ctx, cancel := requestContext(ctx, authorization)
	defer cancel()
	response, err := c.rpc.ListConversations(ctx, &chatv1.ListConversationsRequest{Page: int32(page), Limit: int32(limit)})
	if err != nil {
		return nil, grpcutil.FromStatus(err)
	}
	items := make([]models.ChatConversationView, 0, len(response.GetItems()))
	for _, item := range response.GetItems() {
		items = append(items, models.ChatConversationView{ChatConversation: conversationModel(item), UnreadCount: item.GetUnreadCount(), LastReadSequence: item.GetLastReadSequence()})
	}
	return items, nil
}

func (c *Client) Messages(ctx context.Context, authorization string, conversationID uuid.UUID, after, before *int64, limit int) ([]models.ChatMessage, error) {
	ctx, cancel := requestContext(ctx, authorization)
	defer cancel()
	response, err := c.rpc.ListMessages(ctx, &chatv1.ListMessagesRequest{ConversationId: conversationID.String(), After: after, Before: before, Limit: int32(limit)})
	if err != nil {
		return nil, grpcutil.FromStatus(err)
	}
	items := make([]models.ChatMessage, 0, len(response.GetItems()))
	for _, item := range response.GetItems() {
		items = append(items, messageModel(item))
	}
	return items, nil
}

func (c *Client) Send(ctx context.Context, authorization string, conversationID, clientMessageID uuid.UUID, body string) (models.ChatMessage, error) {
	ctx, cancel := requestContext(ctx, authorization)
	defer cancel()
	response, err := c.rpc.SendMessage(ctx, &chatv1.SendMessageRequest{ConversationId: conversationID.String(), ClientMessageId: clientMessageID.String(), Body: body})
	if err != nil {
		return models.ChatMessage{}, grpcutil.FromStatus(err)
	}
	return messageModel(response), nil
}

func (c *Client) Read(ctx context.Context, authorization string, conversationID uuid.UUID, sequence int64) (models.ChatRead, error) {
	ctx, cancel := requestContext(ctx, authorization)
	defer cancel()
	response, err := c.rpc.MarkRead(ctx, &chatv1.MarkReadRequest{ConversationId: conversationID.String(), Sequence: sequence})
	if err != nil {
		return models.ChatRead{}, grpcutil.FromStatus(err)
	}
	return readModel(response), nil
}

func requestContext(parent context.Context, authorization string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	return grpcutil.WithAuthorization(ctx, authorization), cancel
}

func conversationModel(value *chatv1.Conversation) models.ChatConversation {
	if value == nil {
		return models.ChatConversation{}
	}
	return models.ChatConversation{Base: models.Base{ID: parseUUID(value.GetId()), CreatedAt: protoTime(value.GetCreatedAt()), UpdatedAt: protoTime(value.GetUpdatedAt())}, BuyerID: parseUUID(value.GetBuyerId()), SellerID: parseUUID(value.GetSellerId()), LastSequence: value.GetLastSequence()}
}

func messageModel(value *chatv1.Message) models.ChatMessage {
	if value == nil {
		return models.ChatMessage{}
	}
	return models.ChatMessage{Base: models.Base{ID: parseUUID(value.GetId()), CreatedAt: protoTime(value.GetCreatedAt()), UpdatedAt: protoTime(value.GetUpdatedAt())}, ConversationID: parseUUID(value.GetConversationId()), SenderID: parseUUID(value.GetSenderId()), ClientMessageID: parseUUID(value.GetClientMessageId()), Sequence: value.GetSequence(), Body: value.GetBody()}
}

func readModel(value *chatv1.ReadMarker) models.ChatRead {
	if value == nil {
		return models.ChatRead{}
	}
	return models.ChatRead{ConversationID: parseUUID(value.GetConversationId()), UserID: parseUUID(value.GetUserId()), LastSequence: value.GetLastSequence(), UpdatedAt: protoTime(value.GetUpdatedAt())}
}

func parseUUID(value string) uuid.UUID {
	id, _ := uuid.Parse(value)
	return id
}

func protoTime(value *timestamppb.Timestamp) time.Time {
	if value == nil {
		return time.Time{}
	}
	return value.AsTime().UTC()
}
