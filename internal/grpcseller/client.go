package grpcseller

import (
	"context"
	"encoding/json"
	"time"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	sellerv1 "github.com/example/e-commerce-be/internal/gen/seller/v1"
	"github.com/example/e-commerce-be/internal/grpcutil"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type Client struct {
	rpc         sellerv1.SellerServiceClient
	tokens      coreauth.TokenService
	serviceRole string
}

type Info struct {
	Status         string
	CommissionRate int32
}

type Membership struct {
	SellerID     uuid.UUID
	SellerName   string
	SellerSlug   string
	SellerStatus string
	Role         string
}

type Operations struct {
	Info
	PickupAddress map[string]any
}

func (c *Client) WithServiceIdentity(tokens coreauth.TokenService, role string) *Client {
	return &Client{rpc: c.rpc, tokens: tokens, serviceRole: role}
}

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

func (c *Client) CheckSellerMembership(ctx context.Context, userID, sellerID uuid.UUID) (string, string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := c.rpc.CheckMembership(requestCtx, &sellerv1.CheckMembershipRequest{SellerId: sellerID.String(), UserId: userID.String()})
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

func (c *Client) GetSellerInfo(ctx context.Context, sellerID uuid.UUID) (shared.SellerInfo, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := c.rpc.GetSeller(requestCtx, &sellerv1.GetSellerRequest{SellerId: sellerID.String()})
	if err != nil {
		return shared.SellerInfo{}, grpcutil.FromStatus(err)
	}
	return shared.SellerInfo{Status: result.GetStatus(), CommissionRate: result.GetCommissionRate()}, nil
}

func (c *Client) BatchSellerInfo(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]shared.SellerInfo, error) {
	shops, err := c.Batch(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make(map[uuid.UUID]shared.SellerInfo, len(shops))
	for id, shop := range shops {
		result[id] = shared.SellerInfo{Status: shop.Status, CommissionRate: shop.CommissionRate}
	}
	return result, nil
}

func (c *Client) GetSellerOperations(ctx context.Context, sellerID uuid.UUID) (shared.SellerOperations, error) {
	shop, err := c.GetOperations(ctx, sellerID)
	if err != nil {
		return shared.SellerOperations{}, err
	}
	return shared.SellerOperations{SellerInfo: shared.SellerInfo{Status: shop.Status, CommissionRate: shop.CommissionRate}, PickupAddress: shop.PickupAddress}, nil
}

func (c *Client) CountSellers(ctx context.Context, status string) (int64, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := c.rpc.CountSellers(requestCtx, &sellerv1.CountSellersRequest{Status: status})
	if err != nil {
		return 0, grpcutil.FromStatus(err)
	}
	return result.GetCount(), nil
}

func (c *Client) Batch(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Info, error) {
	if len(ids) == 0 || len(ids) > 100 {
		return nil, shared.ErrInvalid
	}
	raw := make([]string, len(ids))
	for i, id := range ids {
		raw[i] = id.String()
	}
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := c.rpc.BatchGetSellers(requestCtx, &sellerv1.BatchGetSellersRequest{SellerIds: raw})
	if err != nil {
		return nil, grpcutil.FromStatus(err)
	}
	shops := make(map[uuid.UUID]Info, len(result.GetSellers()))
	for _, shop := range result.GetSellers() {
		id, err := uuid.Parse(shop.GetSellerId())
		if err != nil {
			return nil, shared.ErrUnavailable
		}
		shops[id] = Info{Status: shop.GetStatus(), CommissionRate: shop.GetCommissionRate()}
	}
	return shops, nil
}

func (c *Client) Memberships(ctx context.Context) ([]Membership, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := c.rpc.ListMemberships(requestCtx, &sellerv1.ListMembershipsRequest{})
	if err != nil {
		return nil, grpcutil.FromStatus(err)
	}
	items := make([]Membership, 0, len(result.GetMemberships()))
	for _, item := range result.GetMemberships() {
		id, err := uuid.Parse(item.GetSellerId())
		if err != nil {
			return nil, shared.ErrUnavailable
		}
		items = append(items, Membership{SellerID: id, SellerName: item.GetSellerName(), SellerSlug: item.GetSellerSlug(), SellerStatus: item.GetSellerStatus(), Role: item.GetRole()})
	}
	return items, nil
}

func (c *Client) serviceContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if c.serviceRole == "" {
		return nil, nil, shared.ErrUnavailable
	}
	token, err := c.tokens.GenerateService(c.serviceRole, time.Minute)
	if err != nil {
		return nil, nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	return metadata.NewOutgoingContext(requestCtx, metadata.Pairs("authorization", "Bearer "+token)), cancel, nil
}

func (c *Client) Members(ctx context.Context, sellerID uuid.UUID) ([]uuid.UUID, error) {
	requestCtx, cancel, err := c.serviceContext(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	result, err := c.rpc.ListMembers(requestCtx, &sellerv1.ListMembersRequest{SellerId: sellerID.String()})
	if err != nil {
		return nil, grpcutil.FromStatus(err)
	}
	ids := make([]uuid.UUID, 0, len(result.GetUserIds()))
	for _, raw := range result.GetUserIds() {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, shared.ErrUnavailable
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (c *Client) GetOperations(ctx context.Context, sellerID uuid.UUID) (Operations, error) {
	requestCtx, cancel, err := c.serviceContext(ctx)
	if err != nil {
		return Operations{}, err
	}
	defer cancel()
	result, err := c.rpc.GetSellerOperations(requestCtx, &sellerv1.GetSellerOperationsRequest{SellerId: sellerID.String()})
	if err != nil {
		return Operations{}, grpcutil.FromStatus(err)
	}
	var pickup map[string]any
	if err := json.Unmarshal([]byte(result.GetPickupAddressJson()), &pickup); err != nil {
		return Operations{}, shared.ErrUnavailable
	}
	return Operations{Info: Info{Status: result.GetStatus(), CommissionRate: result.GetCommissionRate()}, PickupAddress: pickup}, nil
}
