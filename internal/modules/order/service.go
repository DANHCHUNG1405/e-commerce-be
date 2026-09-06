package order

import (
	"context"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/payment"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/example/e-commerce-be/internal/modules/voucher"
	"github.com/google/uuid"
	"sort"
	"strings"
)

type Service struct {
	repo  *Repository
	sepay payment.Config
}

func New(r *Repository, configs ...payment.Config) *Service {
	s := &Service{repo: r}
	if len(configs) > 0 {
		s.sepay = configs[0]
	}
	return s
}
func (s *Service) Checkout(ctx context.Context, u, a uuid.UUID, key, method string) (models.Order, error) {
	return s.CheckoutSelected(ctx, u, a, key, method, nil)
}

func selectionKey(ids []uuid.UUID) (string, error) {
	if len(ids) > 100 {
		return "", shared.ErrInvalid
	}
	parts := make([]string, len(ids))
	seen := map[uuid.UUID]bool{}
	for i, id := range ids {
		if id == uuid.Nil || seen[id] {
			return "", shared.ErrInvalid
		}
		seen[id] = true
		parts[i] = id.String()
	}
	sort.Strings(parts)
	return strings.Join(parts, ","), nil
}

func (s *Service) CheckoutSelected(ctx context.Context, u, a uuid.UUID, key, method string, ids []uuid.UUID) (models.Order, error) {
	return s.CheckoutVoucher(ctx, u, a, key, method, ids, "")
}
func (s *Service) CheckoutVoucher(ctx context.Context, u, a uuid.UUID, key, method string, ids []uuid.UUID, coupon string) (models.Order, error) {
	if a == uuid.Nil || u == uuid.Nil || strings.TrimSpace(key) == "" || len(key) > 128 || (method != "cod" && method != "sepay") {
		return models.Order{}, shared.ErrInvalid
	}
	if method == "sepay" && !s.sepay.Enabled() {
		return models.Order{}, shared.ErrUnavailable
	}
	selection, err := selectionKey(ids)
	if err != nil {
		return models.Order{}, err
	}
	code, err := voucher.Normalize(coupon)
	if err != nil {
		return models.Order{}, err
	}
	return s.repo.checkout(ctx, u, a, key, method, s.sepay, ids, selection, code)
}
func (s *Service) List(ctx context.Context, u uuid.UUID, p, l int) ([]models.Order, error) {
	return s.repo.List(ctx, u, p, l)
}
func (s *Service) Detail(ctx context.Context, u, id uuid.UUID) (Detail, error) {
	return s.repo.Detail(ctx, u, id)
}
func (s *Service) Cancel(ctx context.Context, u, id uuid.UUID) error {
	return s.repo.Cancel(ctx, u, id)
}
