package httpserver

import (
	coreauth "github.com/example/e-commerce-be/internal/auth"
	"net/http/httptest"
	"testing"
)

func TestDeliveryRoutesRequireAuthentication(t *testing.T) {
	r := NewRouter(nil, coreauth.NewTokenService("test-only"), nil)
	for _, tc := range []struct{ method, path string }{{"POST", "/api/v1/drivers/apply"}, {"GET", "/api/v1/drivers/me"}, {"GET", "/api/v1/admin/drivers"}, {"GET", "/api/v1/drivers/me/shipments"}, {"PUT", "/api/v1/admin/shipments/id/driver"}, {"PATCH", "/api/v1/drivers/me/shipments/id/status"}, {"GET", "/api/v1/shipments/id/events"}, {"GET", "/api/v1/sellers/id/orders/id"}, {"PUT", "/api/v1/sellers/id/profile"}, {"PUT", "/api/v1/sellers/id/members/id"}, {"POST", "/api/v1/sellers/id/orders/id/shipment"}} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != 401 {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
	}
}
