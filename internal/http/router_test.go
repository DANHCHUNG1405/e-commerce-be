package httpserver

import (
	coreauth "github.com/example/e-commerce-be/internal/auth"
	"net/http/httptest"
	"testing"
)

func TestProtectedMarketplaceRoutes(t *testing.T) {
	r := NewRouter(nil, coreauth.NewTokenService("test-only"), nil)
	for _, tc := range []struct{ method, path string }{{"POST", "/api/v1/orders"}, {"GET", "/api/v1/cart"}, {"POST", "/api/v1/sellers"}, {"GET", "/api/v1/wishlist"}, {"POST", "/api/v1/reviews"}, {"PATCH", "/api/v1/users/me"}} {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != 401 {
				t.Fatalf("expected 401, got %d", w.Code)
			}
		})
	}
}
func TestPublicValidation(t *testing.T) {
	r := NewRouter(nil, coreauth.NewTokenService("test-only"), nil)
	for _, path := range []string{"/api/v1/products/not-a-uuid", "/api/v1/products?limit=1000", "/api/v1/categories?page=0", "/api/v1/products?sellerId=invalid", "/api/v1/products?minPrice=-1", "/api/v1/products?sort=invalid"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 400 {
			t.Fatalf("%s: expected 400, got %d", path, w.Code)
		}
	}
}
