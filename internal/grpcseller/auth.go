package grpcseller

import (
	"context"
	"strings"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/grpcutil"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// UnaryAuth permits public, data-limited lookup RPCs; all other methods use
// user JWTs or short-lived service JWTs scoped to one trusted caller.
func UnaryAuth(tokens coreauth.TokenService) grpc.UnaryServerInterceptor {
	userAuth := grpcutil.UnaryAuth(tokens)
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		switch info.FullMethod {
		case "/ecommerce.seller.v1.SellerService/GetSeller", "/ecommerce.seller.v1.SellerService/BatchGetSellers":
			return handler(ctx, request)
		case "/ecommerce.seller.v1.SellerService/ListMembers":
			return serviceAuth(ctx, tokens, "chat", request, handler)
		case "/ecommerce.seller.v1.SellerService/GetSellerOperations":
			return serviceAuth(ctx, tokens, "api", request, handler)
		default:
			return userAuth(ctx, request, info, handler)
		}
	}
}

func serviceAuth(ctx context.Context, tokens coreauth.TokenService, role string, request any, handler grpc.UnaryHandler) (any, error) {
	values := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(values) != 1 {
		return nil, grpcutil.ToStatus(shared.ErrUnauthorized)
	}
	parts := strings.SplitN(values[0], " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return nil, grpcutil.ToStatus(shared.ErrUnauthorized)
	}
	claims, err := tokens.Parse(parts[1])
	if err != nil || claims.Type != "service" || claims.Role != role || claims.Subject != role {
		return nil, grpcutil.ToStatus(shared.ErrUnauthorized)
	}
	return handler(ctx, request)
}
