package httpserver

import (
	"net/http"
	"time"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/http/middleware"
	authmodule "github.com/example/e-commerce-be/internal/modules/auth"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func NewRouter(db *gorm.DB, tokenService coreauth.TokenService) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "timestamp": time.Now().UTC()})
	})
	router.GET("/ready", func(c *gin.Context) {
		sqlDB, err := db.DB()
		if err != nil || sqlDB.PingContext(c.Request.Context()) != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "database unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	v1 := router.Group("/api/v1")
	authService := authmodule.NewService(db, tokenService)
	authHandler := authmodule.NewHandler(authService)
	authRoutes := v1.Group("/auth")
	authRoutes.POST("/register", authHandler.Register)
	authRoutes.POST("/login", authHandler.Login)
	authRoutes.POST("/refresh", authHandler.Refresh)
	authRoutes.POST("/logout", authHandler.Logout)
	authRoutes.GET("/me", middleware.RequireAccessToken(tokenService), authHandler.Me)
	v1.GET("/products", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": []any{}})
	})

	return router
}
