package voucher

import (
	"context"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
	"math/big"
	"regexp"
	"strings"
	"time"
)

type Service struct{ repo *Repository }

func New(r *Repository) *Service { return &Service{repo: r} }

var codePattern = regexp.MustCompile(`^[A-Z0-9_-]{3,40}$`)

func Normalize(code string) (string, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code != "" && !codePattern.MatchString(code) {
		return "", shared.ErrInvalid
	}
	return code, nil
}

type Input struct {
	Code        string    `json:"code"`
	Type        string    `json:"type"`
	Value       int64     `json:"value"`
	MaxDiscount int64     `json:"maxDiscount"`
	MinOrder    int64     `json:"minOrder"`
	UsageLimit  int       `json:"usageLimit"`
	StartsAt    time.Time `json:"startsAt"`
	EndsAt      time.Time `json:"endsAt"`
}

func (in Input) Validate() error {
	code, err := Normalize(in.Code)
	if err != nil || code == "" || in.Value <= 0 || in.Value > 1000000000000 || in.MinOrder < 0 || in.MinOrder > 1000000000000000000 || in.MaxDiscount < 0 || in.MaxDiscount > 1000000000000 || in.UsageLimit < 0 || in.UsageLimit > 1000000000 || in.StartsAt.IsZero() || !in.EndsAt.After(in.StartsAt) {
		return shared.ErrInvalid
	}
	if in.Type != "fixed" && (in.Type != "percent" || in.Value > 100 || in.MaxDiscount == 0) {
		return shared.ErrInvalid
	}
	return nil
}

type Offer struct {
	models.Coupon
	Rules []models.CouponRule `json:"rules"`
}

func (s *Service) Create(ctx context.Context, user uuid.UUID, seller *uuid.UUID, in Input) (Offer, error) {
	if err := in.Validate(); err != nil {
		return Offer{}, err
	}
	if err := s.repo.Manage(ctx, user, seller); err != nil {
		return Offer{}, err
	}
	code, _ := Normalize(in.Code)
	v := models.Coupon{Code: code, Type: in.Type, Value: in.Value, MaxDiscount: in.MaxDiscount, Active: true, StartsAt: in.StartsAt.UTC(), EndsAt: in.EndsAt.UTC(), UsageLimit: in.UsageLimit}
	rule := models.CouponRule{MinOrder: in.MinOrder, SellerID: seller}
	return s.repo.Create(ctx, v, rule)
}
func (s *Service) List(ctx context.Context, seller *uuid.UUID, page, limit int) ([]Offer, error) {
	if page < 1 || page > 100000 || limit < 1 || limit > 100 {
		return nil, shared.ErrInvalid
	}
	return s.repo.List(ctx, seller, page, limit)
}
func (s *Service) SetActive(ctx context.Context, user, id uuid.UUID, active bool) error {
	offer, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if len(offer.Rules) != 1 {
		return shared.ErrConflict
	}
	if err := s.repo.Manage(ctx, user, offer.Rules[0].SellerID); err != nil {
		return err
	}
	return s.repo.SetActive(ctx, id, active)
}

type Line struct {
	SellerID uuid.UUID
	Amount   int64
}
type Quote struct {
	CouponID      uuid.UUID
	Discount      int64
	LineDiscounts []int64
}

type Preview struct {
	Discount   int64  `json:"discount"`
	Subtotal   int64  `json:"subtotal"`
	Total      int64  `json:"total"`
	CouponCode string `json:"couponCode"`
}

func (s *Service) Preview(ctx context.Context, user uuid.UUID, code string, ids []uuid.UUID) (Preview, error) {
	code, err := Normalize(code)
	if err != nil || code == "" || len(ids) > 100 {
		return Preview{}, shared.ErrInvalid
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range ids {
		if id == uuid.Nil || seen[id] {
			return Preview{}, shared.ErrInvalid
		}
		seen[id] = true
	}
	lines, err := s.repo.CartLines(ctx, user, ids)
	if err != nil {
		return Preview{}, err
	}
	offer, err := s.repo.FindCode(ctx, code)
	if err != nil {
		return Preview{}, err
	}
	if len(offer.Rules) != 1 {
		return Preview{}, shared.ErrConflict
	}
	used, err := s.repo.Used(ctx, offer.ID, user)
	if err != nil {
		return Preview{}, err
	}
	if used {
		return Preview{}, shared.ErrConflict
	}
	q, err := Calculate(offer.Coupon, offer.Rules[0], lines, time.Now().UTC())
	if err != nil {
		return Preview{}, err
	}
	var subtotal int64
	for _, l := range lines {
		subtotal += l.Amount
	}
	return Preview{Discount: q.Discount, Subtotal: subtotal, Total: subtotal - q.Discount, CouponCode: code}, nil
}

// Calculate allocates exact integer discounts without overflowing intermediate products.
func Calculate(c models.Coupon, rule models.CouponRule, lines []Line, now time.Time) (Quote, error) {
	if len(lines) > 100 {
		return Quote{}, shared.ErrInvalid
	}
	q := Quote{CouponID: c.ID, LineDiscounts: make([]int64, len(lines))}
	if !c.Active || c.DeletedAt != nil || now.Before(c.StartsAt) || !now.Before(c.EndsAt) || (c.UsageLimit > 0 && c.UsedCount >= c.UsageLimit) {
		return q, shared.ErrConflict
	}
	var eligible int64
	for _, line := range lines {
		if line.Amount < 0 || line.Amount > 10000000000000000 {
			return q, shared.ErrInvalid
		}
		if rule.SellerID == nil || *rule.SellerID == line.SellerID {
			eligible += line.Amount
		}
	}
	if eligible <= 0 || eligible < rule.MinOrder {
		return q, shared.ErrConflict
	}
	d := c.Value
	if c.Type == "percent" {
		if c.Value < 1 || c.Value > 100 || c.MaxDiscount <= 0 {
			return q, shared.ErrInvalid
		}
		d = eligible/100*c.Value + (eligible%100)*c.Value/100
		if d > c.MaxDiscount {
			d = c.MaxDiscount
		}
	} else if c.Type != "fixed" || c.Value <= 0 {
		return q, shared.ErrInvalid
	}
	if d > eligible {
		d = eligible
	}
	q.Discount = d
	remaining, base := d, eligible
	for i, line := range lines {
		if line.Amount == 0 || (rule.SellerID != nil && *rule.SellerID != line.SellerID) {
			continue
		}
		n := new(big.Int).Mul(big.NewInt(remaining), big.NewInt(line.Amount))
		n.Quo(n, big.NewInt(base))
		q.LineDiscounts[i] = n.Int64()
		remaining -= q.LineDiscounts[i]
		base -= line.Amount
	}
	return q, nil
}

// Redeem must be called using a repository bound to the checkout transaction.
func (s *Service) Redeem(ctx context.Context, user, order uuid.UUID, code string, lines []Line) (Quote, error) {
	offer, err := s.repo.LockCode(ctx, code)
	if err != nil {
		return Quote{}, err
	}
	if len(offer.Rules) != 1 {
		return Quote{}, shared.ErrConflict
	}
	quote, err := Calculate(offer.Coupon, offer.Rules[0], lines, time.Now().UTC())
	if err != nil {
		return quote, err
	}
	if err := s.repo.Consume(ctx, offer.Coupon.ID, user, order); err != nil {
		return quote, err
	}
	return quote, nil
}
