package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/database"
	"github.com/example/e-commerce-be/internal/http/middleware"
	"github.com/example/e-commerce-be/internal/http/response"
	"github.com/example/e-commerce-be/internal/modules/recommendation"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	brokers, secret, redisURL := os.Getenv("KAFKA_BROKERS"), os.Getenv("JWT_SECRET"), os.Getenv("REDIS_URL")
	if len(recommendation.BrokerAddresses(brokers)) == 0 || secret == "" || redisURL == "" {
		slog.Error("behavior configuration missing")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	redisClient, err := database.ConnectRedis(ctx, redisURL)
	if err != nil {
		slog.Error("behavior Redis unavailable", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()
	writer := recommendation.NewWriter(brokers, recommendation.RawTopic)
	defer writer.Close()
	collector := &recommendation.Collector{Writer: writer, Redis: redisClient}
	router := gin.New()
	router.Use(gin.Logger(), middleware.Recovery())
	router.GET("/health", func(c *gin.Context) { response.Success(c, 200, gin.H{"status": "ok"}) })
	router.GET("/ready", func(c *gin.Context) {
		checkCtx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if redisClient.Ping(checkCtx).Err() != nil || recommendation.PingAnyBroker(checkCtx, brokers) != nil {
			response.Failure(c, 503, "dependency unavailable")
			return
		}
		response.Success(c, 200, gin.H{"status": "ready"})
	})
	router.POST("/api/v1/behavior/events", middleware.RequireAccessToken(coreauth.NewTokenService(secret)), func(c *gin.Context) {
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
	port := os.Getenv("BEHAVIOR_PORT")
	if port == "" {
		port = "8087"
	}
	server := &http.Server{Addr: ":" + port, Handler: router, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("behavior server failed", "error", err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}
