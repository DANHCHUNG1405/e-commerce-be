package httpserver

import (
	"github.com/example/e-commerce-be/internal/http/response"
	"net/http"
	"time"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/http/api"
	"github.com/example/e-commerce-be/internal/http/middleware"
	authmodule "github.com/example/e-commerce-be/internal/modules/auth"
	"github.com/example/e-commerce-be/internal/modules/cart"
	"github.com/example/e-commerce-be/internal/modules/catalog"
	"github.com/example/e-commerce-be/internal/modules/order"
	"github.com/example/e-commerce-be/internal/modules/payment"
	"github.com/example/e-commerce-be/internal/modules/review"
	"github.com/example/e-commerce-be/internal/modules/seller"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/example/e-commerce-be/internal/modules/shipping"
	"github.com/example/e-commerce-be/internal/modules/user"
	"github.com/example/e-commerce-be/internal/modules/voucher"
	"github.com/example/e-commerce-be/internal/modules/wishlist"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"gorm.io/gorm"
)

func NewRouter(db *gorm.DB, tokenService coreauth.TokenService, redisClient *redis.Client, metadataDB ...*mongo.Database) *gin.Engine {
	return NewRouterWithPayment(db, tokenService, redisClient, payment.Config{}, metadataDB...)
}

func NewRouterWithPayment(db *gorm.DB, tokenService coreauth.TokenService, redisClient *redis.Client, sepay payment.Config, metadataDB ...*mongo.Database) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), middleware.Recovery())
	router.HandleMethodNotAllowed = true
	router.RedirectTrailingSlash = false
	router.NoRoute(func(c *gin.Context) { response.Abort(c, 404, "not found") })
	router.NoMethod(func(c *gin.Context) { response.Abort(c, 405, "method not allowed") })

	router.GET("/health", func(c *gin.Context) {
		response.Success(c, http.StatusOK, gin.H{"status": "ok", "timestamp": time.Now().UTC()})
	})
	router.GET("/ready", func(c *gin.Context) {
		sqlDB, err := db.DB()
		if err != nil || sqlDB.PingContext(c.Request.Context()) != nil || redisClient.Ping(c.Request.Context()).Err() != nil {
			response.Failure(c, http.StatusServiceUnavailable, "dependency unavailable")
			return
		}
		response.Success(c, http.StatusOK, gin.H{"status": "ready"})
	})

	v1 := router.Group("/api/v1")
	authService := authmodule.NewService(db, tokenService, redisClient)
	authHandler := authmodule.NewHandler(authService)
	authRoutes := v1.Group("/auth")
	authRoutes.POST("/register", authHandler.Register)
	authRoutes.POST("/login", authHandler.Login)
	authRoutes.POST("/refresh", authHandler.Refresh)
	authRoutes.POST("/logout", authHandler.Logout)
	authRoutes.GET("/me", middleware.RequireAccessToken(tokenService), authHandler.Me)
	repo := shared.New(db)
	api.DeliveryRoutes(v1, middleware.RequireAccessToken(tokenService), shipping.New(shipping.NewRepository(db)))
	api.SellerManagementRoutes(v1, middleware.RequireAccessToken(tokenService), seller.NewManagement(seller.NewManagementRepository(db)))
	api.VoucherRoutes(v1, middleware.RequireAccessToken(tokenService), voucher.New(voucher.NewRepository(db)))
	api.CatalogRoutes(v1, middleware.RequireAccessToken(tokenService), catalog.New(repo))
	if len(metadataDB) > 0 && metadataDB[0] != nil {
		api.MetadataRoutes(v1, middleware.RequireAccessToken(tokenService), catalog.New(repo), catalog.NewMetadataRepository(metadataDB[0]))
	}
	api.Register(v1, middleware.RequireAccessToken(tokenService), catalog.New(repo), seller.New(repo), user.New(repo), cart.New(cart.NewRepository(db)), order.New(order.NewRepository(db), sepay))
	api.PaymentRoutes(v1, middleware.RequireAccessToken(tokenService), payment.New(payment.NewRepository(db), sepay), sepay)
	api.Commerce(v1, middleware.RequireAccessToken(tokenService), order.New(order.NewRepository(db)), review.New(repo), wishlist.New(db))

	return router
}
