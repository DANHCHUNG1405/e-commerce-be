package grpcutil

import (
	"context"
	"strings"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type userKey struct{}

func UnaryAuth(tokens coreauth.TokenService) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		values := metadata.ValueFromIncomingContext(ctx, "authorization")
		if len(values) != 1 {
			return nil, ToStatus(shared.ErrUnauthorized)
		}
		parts := strings.SplitN(values[0], " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return nil, ToStatus(shared.ErrUnauthorized)
		}
		claims, err := tokens.Parse(parts[1])
		userID, idErr := uuid.Parse(claims.Subject)
		if err != nil || idErr != nil || userID == uuid.Nil || claims.Type != "access" {
			return nil, ToStatus(shared.ErrUnauthorized)
		}
		return handler(context.WithValue(ctx, userKey{}, userID), request)
	}
}

func User(ctx context.Context) (uuid.UUID, error) {
	userID, ok := ctx.Value(userKey{}).(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return uuid.Nil, shared.ErrUnauthorized
	}
	return userID, nil
}

func WithAuthorization(ctx context.Context, authorization string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", authorization)
}
