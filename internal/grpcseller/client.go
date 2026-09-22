package grpcseller

import (
	"context"
	"time"

	sellerv1 "github.com/example/e-commerce-be/internal/gen/seller/v1"
	"github.com/example/e-commerce-be/internal/grpcutil"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct{ rpc sellerv1.SellerServiceClient }

func Dial(target string) (*grpc.ClientConn, *Client, error) {
	connection, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	return connection, &Client{rpc: sellerv1.NewSellerServiceClient(connection)}, nil
}

func (c *Client) CheckMembership(ctx context.Context, sellerID uuid.UUID) (string, string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := c.rpc.CheckMembership(requestCtx, &sellerv1.CheckMembershipRequest{SellerId: sellerID.String()})
	if err != nil {
		return "", "", grpcutil.FromStatus(err)
	}
	return result.GetRole(), result.GetStatus(), nil
}

func (c *Client) GetSeller(ctx context.Context, sellerID uuid.UUID) (string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := c.rpc.GetSeller(requestCtx, &sellerv1.GetSellerRequest{SellerId: sellerID.String()})
	if err != nil {
		return "", grpcutil.FromStatus(err)
	}
	return result.GetStatus(), nil
}
