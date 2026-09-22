package chatserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/events"
	"github.com/example/e-commerce-be/internal/http/middleware"
	"github.com/example/e-commerce-be/internal/http/realtime"
	"github.com/example/e-commerce-be/internal/http/response"
	"github.com/example/e-commerce-be/internal/modules/chat"
	"github.com/example/e-commerce-be/internal/rediskey"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Runtime struct {
	Router  *gin.Engine
	Service *chat.Service

	cancel  context.CancelFunc
	pubsub  *redis.PubSub
	sockets *realtime.Server
	wg      sync.WaitGroup
	once    sync.Once
}

func New(db *gorm.DB, tokens coreauth.TokenService, redisClient *redis.Client, origins []string, users chat.ActiveUserChecker, sellers chat.SellerStatusChecker) *Runtime {
	sockets := realtime.New(tokens, origins)
	service := chat.New(chat.NewRepository(db), sockets, chat.NewRedisLimiter(redisClient), users, sellers)
	sockets.Bind(service)

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
		sqlDB, err := db.DB()
		if err != nil || sqlDB.PingContext(c.Request.Context()) != nil || redisClient.Ping(c.Request.Context()).Err() != nil {
			response.Failure(c, http.StatusServiceUnavailable, "dependency unavailable")
			return
		}
		var schemaReady bool
		if err := db.WithContext(c.Request.Context()).Raw("SELECT to_regclass('chat.chat_conversations') IS NOT NULL AND to_regclass('chat.chat_messages') IS NOT NULL AND to_regclass('chat.chat_reads') IS NOT NULL").Scan(&schemaReady).Error; err != nil || !schemaReady {
			response.Failure(c, http.StatusServiceUnavailable, "dependency unavailable")
			return
		}
		response.Success(c, http.StatusOK, gin.H{"status": "ready"})
	})
	router.GET("/api/v1/ws", gin.WrapH(sockets.Handler()))

	ctx, cancel := context.WithCancel(context.Background())
	pubsub := redisClient.Subscribe(ctx, rediskey.Key("realtime", "notifications"))
	runtime := &Runtime{Router: router, Service: service, cancel: cancel, pubsub: pubsub, sockets: sockets}
	runtime.wg.Add(1)
	go runtime.forwardNotifications(ctx)
	return runtime
}

func (r *Runtime) forwardNotifications(ctx context.Context) {
	defer r.wg.Done()
	channel := r.pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-channel:
			if !ok {
				return
			}
			if err := publishRealtimeNotification(r.sockets, []byte(message.Payload)); err != nil {
				slog.Warn("invalid realtime notification", "error", err)
			}
		}
	}
}

func (r *Runtime) Close() {
	r.once.Do(func() {
		r.cancel()
		_ = r.pubsub.Close()
		r.sockets.Close()
		r.wg.Wait()
	})
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
