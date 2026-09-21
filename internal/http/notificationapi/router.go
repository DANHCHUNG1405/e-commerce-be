package notificationapi

import (
	"context"
	"net/http"
	"time"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/http/api"
	"github.com/example/e-commerce-be/internal/http/middleware"
	"github.com/example/e-commerce-be/internal/http/response"
	"github.com/example/e-commerce-be/internal/modules/notification"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Service interface {
	List(context.Context, uuid.UUID, int, int) ([]notification.Notification, error)
	Unread(context.Context, uuid.UUID) (notification.UnreadCount, error)
	MarkRead(context.Context, uuid.UUID, uuid.UUID) error
	MarkAllRead(context.Context, uuid.UUID) error
}

func NewRouter(service Service, tokens coreauth.TokenService, ready func(context.Context) error) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), middleware.Recovery())
	router.HandleMethodNotAllowed = true
	router.RedirectTrailingSlash = false
	router.NoRoute(func(c *gin.Context) { response.Abort(c, http.StatusNotFound, "not found") })
	router.NoMethod(func(c *gin.Context) { response.Abort(c, http.StatusMethodNotAllowed, "method not allowed") })
	router.GET("/health", func(c *gin.Context) {
		response.Success(c, http.StatusOK, gin.H{"status": "ok", "timestamp": time.Now().UTC()})
	})
	router.GET("/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		if ready == nil || ready(ctx) != nil {
			response.Failure(c, http.StatusServiceUnavailable, "dependency unavailable")
			return
		}
		response.Success(c, http.StatusOK, gin.H{"status": "ready"})
	})

	auth := middleware.RequireAccessToken(tokens)
	v1 := router.Group("/api/v1")
	v1.GET("/notifications", auth, func(c *gin.Context) {
		page, limit, ok := api.Page(c)
		if !ok {
			return
		}
		data, err := service.List(c.Request.Context(), api.User(c), page, limit)
		api.Reply(c, http.StatusOK, data, err)
	})
	v1.GET("/notifications/unread-count", auth, func(c *gin.Context) {
		data, err := service.Unread(c.Request.Context(), api.User(c))
		api.Reply(c, http.StatusOK, data, err)
	})
	v1.PATCH("/notifications/:id/read", auth, func(c *gin.Context) {
		id, ok := api.ID(c, "id")
		if !ok {
			return
		}
		api.Reply(c, http.StatusOK, nil, service.MarkRead(c.Request.Context(), api.User(c), id))
	})
	v1.PATCH("/notifications/read-all", auth, func(c *gin.Context) {
		api.Reply(c, http.StatusOK, nil, service.MarkAllRead(c.Request.Context(), api.User(c)))
	})
	return router
}
