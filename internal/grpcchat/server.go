package grpcchat

import (
	"context"

	chatv1 "github.com/example/e-commerce-be/internal/gen/chat/v1"
	"github.com/example/e-commerce-be/internal/grpcutil"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/chat"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Server struct {
	chatv1.UnimplementedChatServiceServer
	service *chat.Service
}

func NewServer(service *chat.Service) *Server { return &Server{service: service} }

func (s *Server) OpenConversation(ctx context.Context, request *chatv1.OpenConversationRequest) (*chatv1.Conversation, error) {
	userID, err := grpcutil.User(ctx)
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	sellerID, err := parseID(request.GetSellerId())
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	conversation, err := s.service.Open(ctx, userID, sellerID)
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	return conversationProto(conversation, 0, 0), nil
}

func (s *Server) ListConversations(ctx context.Context, request *chatv1.ListConversationsRequest) (*chatv1.ListConversationsResponse, error) {
	userID, err := grpcutil.User(ctx)
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	items, err := s.service.List(ctx, userID, int(request.GetPage()), int(request.GetLimit()))
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	response := &chatv1.ListConversationsResponse{Items: make([]*chatv1.Conversation, 0, len(items))}
	for _, item := range items {
		response.Items = append(response.Items, conversationProto(item.ChatConversation, item.UnreadCount, item.LastReadSequence))
	}
	return response, nil
}

func (s *Server) ListMessages(ctx context.Context, request *chatv1.ListMessagesRequest) (*chatv1.ListMessagesResponse, error) {
	userID, err := grpcutil.User(ctx)
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	conversationID, err := parseID(request.GetConversationId())
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	items, err := s.service.Messages(ctx, userID, conversationID, request.After, request.Before, int(request.GetLimit()))
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	response := &chatv1.ListMessagesResponse{Items: make([]*chatv1.Message, 0, len(items))}
	for _, item := range items {
		response.Items = append(response.Items, messageProto(item))
	}
	return response, nil
}

func (s *Server) SendMessage(ctx context.Context, request *chatv1.SendMessageRequest) (*chatv1.Message, error) {
	userID, err := grpcutil.User(ctx)
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	conversationID, err := parseID(request.GetConversationId())
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	clientMessageID, err := parseID(request.GetClientMessageId())
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	message, err := s.service.Send(ctx, userID, chat.SendInput{ConversationID: conversationID, ClientMessageID: clientMessageID, Body: request.GetBody()})
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	return messageProto(message), nil
}

func (s *Server) MarkRead(ctx context.Context, request *chatv1.MarkReadRequest) (*chatv1.ReadMarker, error) {
	userID, err := grpcutil.User(ctx)
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	conversationID, err := parseID(request.GetConversationId())
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	marker, err := s.service.Read(ctx, userID, conversationID, request.GetSequence())
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	return readProto(marker), nil
}

func parseID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, shared.ErrInvalid
	}
	return id, nil
}

func conversationProto(value models.ChatConversation, unread, lastRead int64) *chatv1.Conversation {
	return &chatv1.Conversation{Id: value.ID.String(), BuyerId: value.BuyerID.String(), SellerId: value.SellerID.String(), LastSequence: value.LastSequence, UnreadCount: unread, LastReadSequence: lastRead, CreatedAt: timestamppb.New(value.CreatedAt), UpdatedAt: timestamppb.New(value.UpdatedAt)}
}

func messageProto(value models.ChatMessage) *chatv1.Message {
	return &chatv1.Message{Id: value.ID.String(), ConversationId: value.ConversationID.String(), SenderId: value.SenderID.String(), ClientMessageId: value.ClientMessageID.String(), Sequence: value.Sequence, Body: value.Body, CreatedAt: timestamppb.New(value.CreatedAt), UpdatedAt: timestamppb.New(value.UpdatedAt)}
}

func readProto(value models.ChatRead) *chatv1.ReadMarker {
	return &chatv1.ReadMarker{ConversationId: value.ConversationID.String(), UserId: value.UserID.String(), LastSequence: value.LastSequence, UpdatedAt: timestamppb.New(value.UpdatedAt)}
}
