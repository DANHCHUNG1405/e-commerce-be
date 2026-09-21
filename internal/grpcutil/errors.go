package grpcutil

import (
	"errors"

	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

func ToStatus(err error) error {
	if err == nil {
		return nil
	}
	code, message := codes.Internal, "internal server error"
	var pg *pgconn.PgError
	switch {
	case errors.Is(err, shared.ErrUnauthorized):
		code, message = codes.Unauthenticated, "unauthorized"
	case errors.Is(err, shared.ErrForbidden):
		code, message = codes.PermissionDenied, "forbidden"
	case errors.Is(err, shared.ErrInvalid):
		code, message = codes.InvalidArgument, "invalid input"
	case errors.Is(err, shared.ErrConflict):
		code, message = codes.AlreadyExists, "conflict"
	case errors.Is(err, shared.ErrRateLimited):
		code, message = codes.ResourceExhausted, "rate limit exceeded"
	case errors.Is(err, shared.ErrUnavailable):
		code, message = codes.Unavailable, "service unavailable"
	case errors.Is(err, gorm.ErrRecordNotFound):
		code, message = codes.NotFound, "not found"
	case errors.As(err, &pg) && pg.Code == "23505":
		code, message = codes.AlreadyExists, "already exists"
	case errors.As(err, &pg) && (pg.Code == "23503" || pg.Code == "23514"):
		code, message = codes.InvalidArgument, "invalid reference or value"
	}
	return status.Error(code, message)
}

func FromStatus(err error) error {
	if err == nil {
		return nil
	}
	switch status.Code(err) {
	case codes.Unauthenticated:
		return shared.ErrUnauthorized
	case codes.PermissionDenied:
		return shared.ErrForbidden
	case codes.InvalidArgument:
		return shared.ErrInvalid
	case codes.AlreadyExists, codes.Aborted, codes.FailedPrecondition:
		return shared.ErrConflict
	case codes.ResourceExhausted:
		return shared.ErrRateLimited
	case codes.NotFound:
		return gorm.ErrRecordNotFound
	case codes.Unavailable, codes.DeadlineExceeded:
		return shared.ErrUnavailable
	default:
		return err
	}
}
