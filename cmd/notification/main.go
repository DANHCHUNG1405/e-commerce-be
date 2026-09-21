package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/database"
	"github.com/example/e-commerce-be/internal/events"
	notificationapi "github.com/example/e-commerce-be/internal/http/notificationapi"
	internalKafka "github.com/example/e-commerce-be/internal/kafka"
	"github.com/example/e-commerce-be/internal/modules/notification"
	"github.com/example/e-commerce-be/internal/rediskey"
	"github.com/joho/godotenv"
)

type config struct {
	databaseURL string
	redisURL    string
	jwtSecret   string
	brokers     string
	dlqTopic    string
	port        string
}

func main() {
	_ = godotenv.Load()
	cfg, err := loadConfig()
	if err != nil {
		slog.Error("invalid notification configuration", "error", err)
		os.Exit(1)
	}
	db, err := database.Connect(cfg.databaseURL)
	if err != nil {
		slog.Error("notification database connection failed", "error", err)
		os.Exit(1)
	}
	if err := database.MigrateNotifications(db); err != nil {
		slog.Error("notification migration failed", "error", err)
		os.Exit(1)
	}
	redisCtx, cancelRedis := context.WithTimeout(context.Background(), 10*time.Second)
	redisClient, err := database.ConnectRedis(redisCtx, cfg.redisURL)
	cancelRedis()
	if err != nil {
		slog.Error("notification Redis connection failed", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(signalCtx)
	defer cancel()
	reader := internalKafka.NewReader(cfg.brokers, events.Topic, "notification-service")
	dlq := internalKafka.NewWriter(cfg.brokers, cfg.dlqTopic)
	defer dlq.Close()
	service := notification.New(db)
	publisher := notification.NewRedisPublisher(rediskey.Key("realtime", "notifications"), func(ctx context.Context, channel string, payload any) error {
		return redisClient.Publish(ctx, channel, payload).Err()
	})
	consumer := notification.NewConsumer(reader, dlq, service, publisher, cfg.dlqTopic)
	ready := func(ctx context.Context) error {
		if err := database.Ping(ctx, db); err != nil {
			return err
		}
		if err := redisClient.Ping(ctx).Err(); err != nil {
			return err
		}
		return internalKafka.Ping(ctx, cfg.brokers)
	}
	router := notificationapi.NewRouter(service, coreauth.NewTokenService(cfg.jwtSecret), ready)
	server := &http.Server{Addr: ":" + cfg.port, Handler: router, ReadHeaderTimeout: 5 * time.Second}
	errorsCh := make(chan error, 2)
	consumerDone := make(chan struct{})
	go func() {
		slog.Info("notification HTTP server listening", "port", cfg.port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorsCh <- fmt.Errorf("notification HTTP server: %w", err)
		}
	}()
	go func() {
		defer close(consumerDone)
		slog.Info("notification consumer started", "brokers", cfg.brokers)
		if err := consumer.Run(ctx); err != nil && ctx.Err() == nil {
			errorsCh <- fmt.Errorf("notification consumer: %w", err)
		}
	}()

	select {
	case <-signalCtx.Done():
	case err := <-errorsCh:
		slog.Error("notification service failed", "error", err)
	}
	cancel()
	if err := reader.Close(); err != nil {
		slog.Error("notification Kafka reader close failed", "error", err)
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("notification HTTP shutdown failed", "error", err)
	}
	select {
	case <-consumerDone:
	case <-shutdownCtx.Done():
		slog.Error("notification consumer shutdown timed out", "error", shutdownCtx.Err())
	}
}

func loadConfig() (config, error) {
	cfg := config{
		databaseURL: os.Getenv("DATABASE_URL"),
		redisURL:    os.Getenv("REDIS_URL"),
		jwtSecret:   os.Getenv("JWT_SECRET"),
		brokers:     value("KAFKA_BROKERS", "kafka:9092"),
		dlqTopic:    value("KAFKA_NOTIFICATION_DLQ_TOPIC", "ecommerce.notification.dlq.v1"),
		port:        value("NOTIFICATION_PORT", "8082"),
	}
	if cfg.databaseURL == "" || cfg.redisURL == "" || cfg.jwtSecret == "" {
		return config{}, errors.New("DATABASE_URL, REDIS_URL and JWT_SECRET are required")
	}
	port, err := strconv.Atoi(cfg.port)
	if err != nil || port < 1 || port > 65535 {
		return config{}, errors.New("NOTIFICATION_PORT must be between 1 and 65535")
	}
	if cfg.brokers == "" || cfg.dlqTopic == "" {
		return config{}, errors.New("Kafka broker and notification DLQ topic are required")
	}
	return cfg, nil
}

func value(key, fallback string) string {
	if result := os.Getenv(key); result != "" {
		return result
	}
	return fallback
}
