package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/modules/payment"
)

func TestRecommendationRoutesRegisterWithMarketplace(t *testing.T) {
	tokens := coreauth.NewTokenService("test-only-secret")
	router := NewRouterWithDependencies(nil, tokens, nil, payment.Config{}, "", "", "", nil, nil)
	AttachBehaviorCollector(router, tokens, nil)
	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/recommendations/home"},
		{http.MethodGet, "/api/v1/products/00000000-0000-4000-8000-000000000001/recommendations"},
		{http.MethodPost, "/api/v1/behavior/events"},
	}
	for _, route := range routes {
		request := httptest.NewRequest(route.method, route.path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s returned %d; want JWT enforcement", route.path, response.Code)
		}
	}
}
