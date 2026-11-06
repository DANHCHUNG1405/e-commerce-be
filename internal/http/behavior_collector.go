package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/http/middleware"
	"github.com/example/e-commerce-be/internal/http/response"
	"github.com/example/e-commerce-be/internal/modules/recommendation"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func AttachBehaviorCollector(router *gin.Engine, tokens coreauth.TokenService, collector *recommendation.Collector) {
	endpoint := router.Group("/api/v1/behavior", middleware.RequireAccessToken(tokens))
	endpoint.POST("/events", func(c *gin.Context) {
		if collector == nil {
			response.Failure(c, 503, "behavior collection unavailable")
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		decoder := json.NewDecoder(c.Request.Body)
		decoder.DisallowUnknownFields()
		var input struct {
			Events []recommendation.InputEvent `json:"events"`
		}
		if err := decoder.Decode(&input); err != nil {
			response.Failure(c, 400, "invalid behavior request")
			return
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			response.Failure(c, 400, "invalid behavior request")
			return
		}
		userID, err := uuid.Parse(c.GetString("user_id"))
		if err != nil {
			response.Failure(c, 401, "invalid access token")
			return
		}
		publishCtx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Second)
		defer cancel()
		err = collector.Collect(publishCtx, userID, input.Events)
		switch {
		case err == nil:
			response.Success(c, 202, gin.H{"accepted": len(input.Events)})
		case errors.Is(err, recommendation.ErrInvalidEvent):
			response.Failure(c, 400, "invalid behavior event")
		case errors.Is(err, recommendation.ErrRateLimited):
			response.Failure(c, 429, "behavior rate limit exceeded")
		default:
			slog.Error("behavior collection failed", "error", err)
			response.Failure(c, 503, "behavior collection unavailable")
		}
	})
}
