package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/events"
	"github.com/example/e-commerce-be/internal/http/api"
	"github.com/example/e-commerce-be/internal/http/middleware"
	"github.com/example/e-commerce-be/internal/http/realtime"
	"github.com/example/e-commerce-be/internal/modules/chat"
	"github.com/example/e-commerce-be/internal/rediskey"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func AttachChat(router *gin.Engine, db *gorm.DB, tokens coreauth.TokenService, redisClient *redis.Client, origins []string) func() {
	sockets := realtime.New(tokens, origins)
	service := chat.New(chat.NewRepository(db), sockets, chat.NewRedisLimiter(redisClient))
	sockets.Bind(service)
	api.ChatRoutes(router.Group("/api/v1"), middleware.RequireAccessToken(tokens), service)
	router.GET("/api/v1/ws", gin.WrapH(sockets.Handler()))

	ctx, cancel := context.WithCancel(context.Background())
	pubsub := redisClient.Subscribe(ctx, rediskey.Key("realtime", "notifications"))
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for message := range pubsub.Channel() {
			if err := publishRealtimeNotification(sockets, []byte(message.Payload)); err != nil {
				slog.Warn("invalid realtime notification", "error", err)
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			_ = pubsub.Close()
			sockets.Close()
			wg.Wait()
		})
	}
}

type notificationSocketPublisher interface {
	Publish([]uuid.UUID, string, any)
}

func publishRealtimeNotification(publisher notificationSocketPublisher, body []byte) error {
	var event events.RealtimeNotification
	if err := json.Unmarshal(body, &event); err != nil {
		return err
	}
	if event.RecipientUserID == uuid.Nil || len(event.Notification) == 0 || !json.Valid(event.Notification) {
		return errors.New("invalid realtime notification payload")
	}
	publisher.Publish([]uuid.UUID{event.RecipientUserID}, "notification:created", event.Notification)
	return nil
}
