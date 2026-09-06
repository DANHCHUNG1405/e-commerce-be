package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/example/e-commerce-be/internal/http/response"
	"github.com/example/e-commerce-be/internal/modules/payment"
	"github.com/gin-gonic/gin"
)

func PaymentRoutes(v *gin.RouterGroup, auth gin.HandlerFunc, service *payment.Service, config payment.Config) {
	v.GET("/admin/payments/sepay/receipts", auth, func(c *gin.Context) {
		p, l, ok := Page(c)
		if !ok {
			return
		}
		data, err := service.Receipts(c.Request.Context(), User(c), c.Query("status"), p, l)
		Reply(c, 200, data, err)
	})
	v.GET("/orders/:id/payment", auth, func(c *gin.Context) {
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		data, err := service.Instructions(c.Request.Context(), User(c), id)
		Reply(c, 200, data, err)
	})
	v.POST("/payments/sepay/webhook", func(c *gin.Context) {
		if !config.Enabled() {
			response.Failure(c, 503, "payment service unavailable")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10))
		if err != nil {
			response.Failure(c, 400, "invalid webhook body")
			return
		}
		if !config.Verify(body, c.GetHeader("X-SePay-Timestamp"), c.GetHeader("X-SePay-Signature"), time.Now()) {
			response.Failure(c, 401, "invalid webhook signature")
			return
		}
		var event payment.Webhook
		if json.Unmarshal(body, &event) != nil {
			response.Failure(c, 400, "invalid webhook payload")
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()
		if err := service.Receive(ctx, event); err != nil {
			Reply(c, 200, nil, err)
			return
		}
		response.WebhookSuccess(c)
	})
}
