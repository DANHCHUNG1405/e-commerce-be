package httpserver

import (
	"encoding/json"
	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/http/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestApplicationResponseEnvelope(t *testing.T) {
	tokens := coreauth.NewTokenService("test-only")
	r := NewRouter(nil, tokens, nil)
	r.GET("/test-panic", func(c *gin.Context) { panic("sensitive panic text") })
	access, err := tokens.Generate(uuid.New(), "customer", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path, body, token string
		status                    int
	}{
		{"GET", "/health", "", "", 200},
		{"GET", "/api/v1/cart", "", "", 401},
		{"GET", "/api/v1/cart", "", "invalid", 401},
		{"GET", "/does-not-exist", "", "", 404},
		{"POST", "/health", "", "", 405},
		{"GET", "/health/", "", "", 404},
		{"GET", "/test-panic", "", "", 500},
		{"GET", "/api/v1/products?limit=0", "", "", 400},
		{"GET", "/api/v1/products/invalid", "", "", 400},
		{"POST", "/api/v1/auth/login", "{", "", 400},
		{"POST", "/api/v1/auth/logout", "{}", "", 400},
		{"POST", "/api/v1/auth/logout", `{"refreshToken":"invalid"}`, "", 401},
		{"PATCH", "/api/v1/users/me", "{}", access, 400},
	} {
		t.Run(tc.method+tc.path+tc.body+strings.Repeat("_", len(tc.token)%3), func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			r.ServeHTTP(w, req)
			var e response.Envelope
			if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
				t.Fatalf("not JSON: %s", w.Body.String())
			}
			if w.Code != tc.status || e.StatusCode != w.Code || e.Error != (w.Code >= 400) || e.Data.Msg == "" {
				t.Fatalf("invalid envelope %s", w.Body.String())
			}
			if e.Data.Content == nil {
				t.Fatal("content must not be null")
			}
			if len(e.ResponseTimestamp) != 24 {
				t.Fatal("invalid timestamp")
			}
			if strings.Contains(w.Body.String(), "sensitive panic text") {
				t.Fatal("panic leaked")
			}
		})
	}
}
