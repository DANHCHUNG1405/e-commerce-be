package shipping

import (
	"context"
	"errors"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
	"testing"
)

func TestTransitions(t *testing.T) {
	allowed := map[string]bool{"assigned:accepted": true, "accepted:picked_up": true, "picked_up:delivering": true, "delivering:delivered": true, "delivering:failed": true, "failed:delivering": true, "failed:returned": true}
	states := []string{"pending", "assigned", "accepted", "picked_up", "delivering", "failed", "returned", "delivered", "cancelled"}
	for _, from := range states {
		for _, to := range states {
			if Transition(from, to) != allowed[from+":"+to] {
				t.Fatalf("transition %s -> %s", from, to)
			}
		}
	}
}
func TestRejectInvalidBeforeIO(t *testing.T) {
	s := New(nil)
	u := uuid.New()
	ctx := context.Background()
	for _, in := range []Update{{Status: "delivered"}, {Status: "failed"}, {Status: "accepted", CODCollected: true}, {Status: "invented"}} {
		key := "key"
		if in.Status == "delivered" {
			key = ""
		}
		if err := s.Update(ctx, u, u, key, in); !errors.Is(err, shared.ErrInvalid) {
			t.Fatal("invalid update accepted")
		}
	}
	if _, err := s.Apply(ctx, u, Application{}); !errors.Is(err, shared.ErrInvalid) {
		t.Fatal("empty profile accepted")
	}
	if _, err := s.List(ctx, u, false, "", 0, 20); !errors.Is(err, shared.ErrInvalid) {
		t.Fatal("invalid page")
	}
}
