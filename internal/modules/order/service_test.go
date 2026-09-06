package order

import (
	"context"
	"errors"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
	"testing"
)

func TestCheckoutValidation(t *testing.T) {
	s := New(nil)
	for _, tc := range []struct{ key, method string }{{"", "cod"}, {"k", "online"}} {
		_, err := s.Checkout(context.Background(), uuid.New(), uuid.New(), tc.key, tc.method)
		if !errors.Is(err, shared.ErrInvalid) {
			t.Fatal("invalid checkout accepted")
		}
	}
}
func TestStatusTransitions(t *testing.T) {
	for _, tc := range []struct {
		from, to string
		want     bool
	}{{"pending", "confirmed", true}, {"confirmed", "shipping", true}, {"shipping", "delivered", true}, {"pending", "delivered", false}, {"delivered", "shipping", false}, {"cancelled", "confirmed", false}} {
		if validTransition(tc.from, tc.to) != tc.want {
			t.Fatalf("unexpected transition %s -> %s", tc.from, tc.to)
		}
	}
}

func TestSelectionKey(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	x, err := selectionKey([]uuid.UUID{a, b})
	if err != nil {
		t.Fatal(err)
	}
	y, err := selectionKey([]uuid.UUID{b, a})
	if err != nil || x != y {
		t.Fatal("selection order changed idempotency")
	}
	for _, ids := range [][]uuid.UUID{{a, a}, {uuid.Nil}, make([]uuid.UUID, 101)} {
		if _, err := selectionKey(ids); !errors.Is(err, shared.ErrInvalid) {
			t.Fatal("invalid selection accepted")
		}
	}
	if x, err := selectionKey(nil); err != nil || x != "" {
		t.Fatal("whole cart compatibility failed")
	}
}
