package grpcseller

import (
	"context"

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
	if _, err := grpcutil.User(ctx); err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	id, err := parseSellerID(request.GetSellerId())
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	shop, err := s.repo.Profile(ctx, id)
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	return &sellerv1.GetSellerResponse{SellerId: shop.ID.String(), Status: shop.Status}, nil
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
