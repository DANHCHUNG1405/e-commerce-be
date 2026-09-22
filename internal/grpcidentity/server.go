package grpcidentity

import (
	"context"
	"errors"

	identityv1 "github.com/example/e-commerce-be/internal/gen/identity/v1"
	"github.com/example/e-commerce-be/internal/grpcutil"
	identityauth "github.com/example/e-commerce-be/internal/modules/auth"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"
	"gorm.io/gorm"
)

type Server struct {
	identityv1.UnimplementedIdentityServiceServer
	auth *identityauth.Service
}

func NewServer(auth *identityauth.Service) *Server { return &Server{auth: auth} }

func (s *Server) CheckUserActive(ctx context.Context, request *identityv1.CheckUserActiveRequest) (*emptypb.Empty, error) {
	caller, err := grpcutil.User(ctx)
	if err != nil {
		return nil, grpcutil.ToStatus(err)
	}
	userID, err := uuid.Parse(request.GetUserId())
	if err != nil || userID == uuid.Nil {
		return nil, grpcutil.ToStatus(shared.ErrInvalid)
	}
	if caller != userID {
		return nil, grpcutil.ToStatus(shared.ErrForbidden)
	}
	if _, err := s.auth.FindUser(ctx, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = shared.ErrForbidden
		}
		return nil, grpcutil.ToStatus(err)
	}
	return &emptypb.Empty{}, nil
}
