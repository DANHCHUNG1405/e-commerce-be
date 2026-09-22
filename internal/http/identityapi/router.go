package identityapi

import (
	"context"
	"net/http"
	"time"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/http/api"
	"github.com/example/e-commerce-be/internal/http/middleware"
	"github.com/example/e-commerce-be/internal/http/response"
	identityauth "github.com/example/e-commerce-be/internal/modules/auth"
	"github.com/example/e-commerce-be/internal/modules/user"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func NewRouter(authService *identityauth.Service, userService *user.Service, tokens coreauth.TokenService, ready func(context.Context) error) *gin.Engine {
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
	authHandler := identityauth.NewHandler(authService)
	authRoutes := v1.Group("/auth")
	authRoutes.POST("/register", authHandler.Register)
	authRoutes.POST("/login", authHandler.Login)
	authRoutes.POST("/refresh", authHandler.Refresh)
	authRoutes.POST("/logout", authHandler.Logout)
	authRoutes.POST("/forgot-password", authHandler.ForgotPassword)
	authRoutes.POST("/reset-password", authHandler.ResetPassword)
	protectedAuth := authRoutes.Group("", middleware.RequireAccessToken(tokens))
	protectedAuth.POST("/change-password", authHandler.ChangePassword)
	protectedAuth.GET("/me", authHandler.Me)

	users := v1.Group("/users/me", middleware.RequireAccessToken(tokens))
	users.GET("/permissions", func(c *gin.Context) {
		data, err := userService.Permissions(c.Request.Context(), api.User(c))
		api.Reply(c, http.StatusOK, data, err)
	})
	users.GET("/seller-memberships", func(c *gin.Context) {
		data, err := userService.SellerMemberships(c.Request.Context(), api.User(c))
		api.Reply(c, http.StatusOK, data, err)
	})
	users.PATCH("", func(c *gin.Context) {
		var input struct {
			FullName string `json:"fullName" binding:"required,max=200"`
		}
		if !api.Bind(c, &input) {
			return
		}
		api.Reply(c, http.StatusOK, nil, userService.Profile(c.Request.Context(), api.User(c), input.FullName))
	})
	users.GET("/addresses", func(c *gin.Context) {
		page, limit, ok := api.Page(c)
		if !ok {
			return
		}
		data, err := userService.Addresses(c.Request.Context(), api.User(c), page, limit)
		api.Reply(c, http.StatusOK, data, err)
	})
	users.POST("/addresses", func(c *gin.Context) {
		var input user.AddressInput
		if !api.Bind(c, &input) {
			return
		}
		data, err := userService.SaveAddress(c.Request.Context(), api.User(c), uuid.Nil, input)
		api.Reply(c, http.StatusCreated, data, err)
	})
	users.PUT("/addresses/:id", func(c *gin.Context) {
		id, ok := api.ID(c, "id")
		if !ok {
			return
		}
		var input user.AddressInput
		if !api.Bind(c, &input) {
			return
		}
		data, err := userService.SaveAddress(c.Request.Context(), api.User(c), id, input)
		api.Reply(c, http.StatusOK, data, err)
	})
	users.DELETE("/addresses/:id", func(c *gin.Context) {
		id, ok := api.ID(c, "id")
		if !ok {
			return
		}
		api.Reply(c, http.StatusOK, nil, userService.DeleteAddress(c.Request.Context(), api.User(c), id))
	})
	return router
}
