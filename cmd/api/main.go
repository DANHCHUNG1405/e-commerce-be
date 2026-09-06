package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/config"
	"github.com/example/e-commerce-be/internal/database"
	httpserver "github.com/example/e-commerce-be/internal/http"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}

	if err := database.Migrate(db); err != nil {
		slog.Error("database migration failed", "error", err)
		os.Exit(1)
	}

	mongoClient, err := database.ConnectMongo(context.Background(), cfg.MongoURL)
	if err != nil {
		slog.Error("mongo connection failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = mongoClient.Disconnect(context.Background()) }()

	redisCtx, cancelRedis := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelRedis()
	redisClient, err := database.ConnectRedis(redisCtx, cfg.RedisURL)
	if err != nil {
		slog.Error("Redis connection failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = redisClient.Close() }()

	tokens := coreauth.NewTokenService(cfg.JWTSecret)
	router := httpserver.NewRouterWithPayment(db, tokens, redisClient, cfg.SePay, mongoClient.Database(cfg.MongoDatabase))
	closeChat := httpserver.AttachChat(router, db, tokens, redisClient, cfg.WebSocketOrigins)
	defer closeChat()
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("API is listening", "port", cfg.Port, "environment", cfg.Environment)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	closeChat()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
	}
}
