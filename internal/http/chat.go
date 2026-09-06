package httpserver

import (
	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/http/api"
	"github.com/example/e-commerce-be/internal/http/middleware"
	"github.com/example/e-commerce-be/internal/http/realtime"
	"github.com/example/e-commerce-be/internal/modules/chat"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func AttachChat(router *gin.Engine, db *gorm.DB, tokens coreauth.TokenService, redisClient *redis.Client, origins []string) func() {
	sockets := realtime.New(tokens, origins)
	service := chat.New(chat.NewRepository(db), sockets, chat.NewRedisLimiter(redisClient))
	sockets.Bind(service)
	api.ChatRoutes(router.Group("/api/v1"), middleware.RequireAccessToken(tokens), service)
	router.GET("/api/v1/ws", gin.WrapH(sockets.Handler()))
	return sockets.Close
}
