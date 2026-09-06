package voucher

import (
	"github.com/example/e-commerce-be/internal/models"
	"github.com/google/uuid"
	"math/rand"
	"testing"
	"time"
)

func TestCalculate(t *testing.T) {
	now := time.Now().UTC()
	a, b := uuid.New(), uuid.New()
	c := models.Coupon{Active: true, Type: "percent", Value: 10, MaxDiscount: 30000, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour)}
	lines := []Line{{a, 100000}, {b, 200000}, {a, 400000}}
	q, err := Calculate(c, models.CouponRule{SellerID: &a, MinOrder: 100000}, lines, now)
	if err != nil || q.Discount != 30000 || q.LineDiscounts[1] != 0 || q.LineDiscounts[0]+q.LineDiscounts[2] != 30000 {
		t.Fatal("wrong seller discount", err)
	}
	for _, kind := range []string{"inactive", "expired", "not started", "quota", "minimum", "wrong seller"} {
		t.Run(kind, func(t *testing.T) {
			x := c
			rule := models.CouponRule{}
			switch kind {
			case "inactive":
				x.Active = false
			case "expired":
				x.EndsAt = now
			case "not started":
				x.StartsAt = now.Add(time.Hour)
			case "quota":
				x.UsageLimit = 1
				x.UsedCount = 1
			case "minimum":
				rule.MinOrder = 1000000
			case "wrong seller":
				id := uuid.New()
				rule.SellerID = &id
			}
			if _, err := Calculate(x, rule, lines, now); err == nil {
				t.Fatal("invalid voucher accepted")
			}
		})
	}
	c.Type = "fixed"
	c.Value = 1000000
	q, err = Calculate(c, models.CouponRule{}, []Line{{a, 1}, {b, 2}}, now)
	if err != nil || q.Discount != 3 {
		t.Fatal("discount exceeds subtotal")
	}
}
func TestAllocationInvariant(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	now := time.Now()
	for n := 0; n < 300; n++ {
		lines := make([]Line, 1+r.Intn(100))
		var total int64
		for i := range lines {
			lines[i].Amount = r.Int63n(10000000000000000)
			total += lines[i].Amount
		}
		c := models.Coupon{Active: true, Type: "fixed", Value: r.Int63n(1000000000000) + 1, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour)}
		q, err := Calculate(c, models.CouponRule{}, lines, now)
		if err != nil {
			t.Fatal(err)
		}
		var sum int64
		for i, d := range q.LineDiscounts {
			if d < 0 || d > lines[i].Amount {
				t.Fatal("invalid allocation")
			}
			sum += d
		}
		if sum != q.Discount || sum > total {
			t.Fatal("allocation does not balance")
		}
	}
}
func TestInputValidation(t *testing.T) {
	now := time.Now()
	in := Input{Code: "SALE10", Type: "percent", Value: 10, MaxDiscount: 1000, StartsAt: now, EndsAt: now.Add(time.Hour)}
	if err := in.Validate(); err != nil {
		t.Fatal(err)
	}
	in.MaxDiscount = 0
	if in.Validate() == nil {
		t.Fatal("uncapped percent accepted")
	}
	if code, err := Normalize(" sale10 "); err != nil || code != "SALE10" {
		t.Fatal("code normalization failed")
	}
	if _, err := Normalize("x;select"); err == nil {
		t.Fatal("invalid code accepted")
	}
}
