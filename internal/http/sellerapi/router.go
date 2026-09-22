package sellerapi

import (
	"context"
	"net/http"
	"time"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/http/api"
	"github.com/example/e-commerce-be/internal/http/middleware"
	"github.com/example/e-commerce-be/internal/http/response"
	"github.com/example/e-commerce-be/internal/modules/seller"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func NewRouter(db *gorm.DB, tokens coreauth.TokenService, ready func(context.Context) error) *gin.Engine {
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
		if err := ready(c.Request.Context()); err != nil {
			response.Failure(c, http.StatusServiceUnavailable, "dependency unavailable")
			return
		}
		response.Success(c, http.StatusOK, gin.H{"status": "ready"})
	})
	v1 := router.Group("/api/v1")
	auth := middleware.RequireAccessToken(tokens)
	api.SellerCoreRoutes(v1, auth, seller.New(shared.New(db)))
	api.SellerOwnedManagementRoutes(v1, auth, seller.NewManagement(seller.NewManagementRepository(db)))
	return router
}
