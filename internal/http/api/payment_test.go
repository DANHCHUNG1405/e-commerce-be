package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"github.com/example/e-commerce-be/internal/modules/payment"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWebhookRejectsInvalidRequests(t *testing.T) {
	cfg := payment.Config{Bank: "TestBank", Account: "test-account", WebhookSecret: strings.Repeat("x", 32)}
	for _, tc := range []struct {
		name, body string
		signed     bool
		want       int
	}{
		{"unsigned", `{"id":1}`, false, 401}, {"signed invalid json", `{`, true, 400}, {"invalid fields", `{"id":0}`, true, 400}, {"oversized", strings.Repeat("x", 65537), false, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			PaymentRoutes(r.Group("/api/v1"), func(c *gin.Context) { c.AbortWithStatus(401) }, payment.New(nil, cfg), cfg)
			req := httptest.NewRequest("POST", "/api/v1/payments/sepay/webhook", bytes.NewBufferString(tc.body))
			if tc.signed {
				ts := strconv.FormatInt(time.Now().Unix(), 10)
				mac := hmac.New(sha256.New, []byte(cfg.WebhookSecret))
				mac.Write([]byte(ts + "." + tc.body))
				req.Header.Set("X-SePay-Timestamp", ts)
				req.Header.Set("X-SePay-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d", w.Code, tc.want)
			}
		})
	}
}
