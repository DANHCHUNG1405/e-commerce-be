package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/modules/payment"
)

func TestRecommendationRoutesRegisterWithMarketplace(t *testing.T) {
	router := NewRouterWithDependencies(nil, coreauth.NewTokenService("test-only-secret"), nil, payment.Config{}, "", "", "", nil, nil)
	for _, path := range []string{"/api/v1/recommendations/home", "/api/v1/products/00000000-0000-4000-8000-000000000001/recommendations"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s returned %d; want JWT enforcement", path, response.Code)
		}
	}
}
