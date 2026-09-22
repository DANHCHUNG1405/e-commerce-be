package grpcidentity

import (
	"context"
	"time"

	identityv1 "github.com/example/e-commerce-be/internal/gen/identity/v1"
	"github.com/example/e-commerce-be/internal/grpcutil"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	rpc identityv1.IdentityServiceClient
}

func Dial(target string) (*grpc.ClientConn, *Client, error) {
	connection, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	return connection, &Client{rpc: identityv1.NewIdentityServiceClient(connection)}, nil
}

func (c *Client) Active(ctx context.Context, userID uuid.UUID) error {
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := c.rpc.CheckUserActive(requestCtx, &identityv1.CheckUserActiveRequest{UserId: userID.String()})
	return grpcutil.FromStatus(err)
}
