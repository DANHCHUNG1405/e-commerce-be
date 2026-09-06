package catalog

import (
	"context"
	"errors"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
	"testing"
)

func TestFilterValidation(t *testing.T) {
	negative, low, high := int64(-1), int64(1), int64(2)
	zero := uuid.Nil
	for _, f := range []Filter{{Sort: "price; DROP TABLE products"}, {MinPrice: &negative}, {MinPrice: &high, MaxPrice: &low}, {SellerID: &zero}} {
		if _, err := New(nil).Search(context.Background(), f, 1, 20); !errors.Is(err, shared.ErrInvalid) {
			t.Fatal("invalid filter accepted")
		}
	}
	for _, sort := range []string{"", "newest", "price_asc", "price_desc", "best_selling", "rating"} {
		if err := (Filter{Sort: sort}).Validate(); err != nil {
			t.Fatal(err)
		}
	}
}
