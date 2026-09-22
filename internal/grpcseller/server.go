package grpcseller

import (
	"context"
	"encoding/json"

	sellerv1 "github.com/example/e-commerce-be/internal/gen/seller/v1"
	"github.com/example/e-commerce-be/internal/grpcutil"
	"github.com/example/e-commerce-be/internal/modules/seller"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Server struct {
	sellerv1.UnimplementedSellerServiceServer
	repo *seller.ManagementRepository
}

func NewServer(db *gorm.DB) *Server { return &Server{repo: seller.NewManagementRepository(db)} }

func parseSellerID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, shared.ErrInvalid
	}
	return id, nil
}

func (s *Server) GetSeller(ctx context.Context, request *sellerv1.GetSellerRequest) (*sellerv1.GetSellerResponse, error) {
	id, err := parseSellerID(request.GetSellerId())
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	shop, err := s.repo.Profile(ctx, id)
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	return &sellerv1.GetSellerResponse{SellerId: shop.ID.String(), Status: shop.Status, CommissionRate: int32(shop.CommissionRate)}, nil
}

func (s *Server) BatchGetSellers(ctx context.Context, request *sellerv1.BatchGetSellersRequest) (*sellerv1.BatchGetSellersResponse, error) {
	if len(request.GetSellerIds()) == 0 || len(request.GetSellerIds()) > 100 {
		return nil, grpcutil.ToStatus(shared.ErrInvalid)
	}
	ids := make([]uuid.UUID, 0, len(request.GetSellerIds()))
	seen := make(map[uuid.UUID]bool, len(request.GetSellerIds()))
	for _, raw := range request.GetSellerIds() {
		id, err := parseSellerID(raw)
		if err != nil || seen[id] {
			return nil, grpcutil.ToStatus(shared.ErrInvalid)
		}
		seen[id] = true
		ids = append(ids, id)
	}
	shops, err := s.repo.Batch(ctx, ids)
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	response := &sellerv1.BatchGetSellersResponse{Sellers: make([]*sellerv1.GetSellerResponse, 0, len(shops))}
	for _, shop := range shops {
		response.Sellers = append(response.Sellers, &sellerv1.GetSellerResponse{SellerId: shop.ID.String(), Status: shop.Status, CommissionRate: int32(shop.CommissionRate)})
	}
	return response, nil
}

func (s *Server) GetSellerOperations(ctx context.Context, request *sellerv1.GetSellerOperationsRequest) (*sellerv1.GetSellerOperationsResponse, error) {
	id, err := parseSellerID(request.GetSellerId())
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	shop, err := s.repo.Profile(ctx, id)
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	pickup, err := json.Marshal(shop.PickupAddress)
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	return &sellerv1.GetSellerOperationsResponse{SellerId: shop.ID.String(), Status: shop.Status, CommissionRate: int32(shop.CommissionRate), PickupAddressJson: string(pickup)}, nil
}

func (s *Server) ListMemberships(ctx context.Context, _ *sellerv1.ListMembershipsRequest) (*sellerv1.ListMembershipsResponse, error) {
	user, err := grpcutil.User(ctx)
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	items, err := s.repo.Memberships(ctx, user)
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	response := &sellerv1.ListMembershipsResponse{Memberships: make([]*sellerv1.Membership, 0, len(items))}
	for _, item := range items {
		response.Memberships = append(response.Memberships, &sellerv1.Membership{SellerId: item.SellerID.String(), SellerName: item.SellerName, SellerSlug: item.SellerSlug, SellerStatus: item.SellerStatus, Role: item.Role})
	}
	return response, nil
}

func (s *Server) ListMembers(ctx context.Context, request *sellerv1.ListMembersRequest) (*sellerv1.ListMembersResponse, error) {
	id, err := parseSellerID(request.GetSellerId())
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	ids, err := s.repo.MemberIDs(ctx, id)
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	response := &sellerv1.ListMembersResponse{UserIds: make([]string, 0, len(ids))}
	for _, id := range ids {
		response.UserIds = append(response.UserIds, id.String())
	}
	return response, nil
}

func (s *Server) CountSellers(ctx context.Context, request *sellerv1.CountSellersRequest) (*sellerv1.CountSellersResponse, error) {
	user, err := grpcutil.User(ctx)
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	if err := s.repo.Admin(ctx, user); err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	if request.GetStatus() != "pending" {
		return nil, grpcutil.ToStatus(shared.ErrInvalid)
	}
	count, err := s.repo.CountStatus(ctx, request.GetStatus())
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	return &sellerv1.CountSellersResponse{Count: count}, nil
}

func (s *Server) CheckMembership(ctx context.Context, request *sellerv1.CheckMembershipRequest) (*sellerv1.CheckMembershipResponse, error) {
	user, err := grpcutil.User(ctx)
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	id, err := parseSellerID(request.GetSellerId())
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	if request.GetUserId() != "" && request.GetUserId() != user.String() {
		return nil, grpcutil.ToStatus(shared.ErrForbidden)
	}
	role, err := s.repo.Role(ctx, user, id, false)
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	shop, err := s.repo.Profile(ctx, id)
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	return &sellerv1.CheckMembershipResponse{Role: role, Status: shop.Status}, nil
}
