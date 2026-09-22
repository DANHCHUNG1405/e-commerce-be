package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/database"
	identityv1 "github.com/example/e-commerce-be/internal/gen/identity/v1"
	"github.com/example/e-commerce-be/internal/grpcidentity"
	"github.com/example/e-commerce-be/internal/grpcutil"
	"github.com/example/e-commerce-be/internal/http/identityapi"
	"github.com/example/e-commerce-be/internal/mailer"
	identityauth "github.com/example/e-commerce-be/internal/modules/auth"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/example/e-commerce-be/internal/modules/user"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
)

type config struct {
	databaseURL      string
	redisURL         string
	jwtSecret        string
	httpPort         string
	grpcPort         string
	smtpHost         string
	smtpPort         int
	smtpUser         string
	smtpPassword     string
	smtpFrom         string
	passwordResetURL string
}

func main() {
	_ = godotenv.Load()
	cfg, err := loadConfig()
	if err != nil {
		slog.Error("invalid identity configuration", "error", err)
		os.Exit(1)
	}
	db, err := database.Connect(cfg.databaseURL)
	if err != nil {
		slog.Error("identity database connection failed", "error", err)
		os.Exit(1)
	}
	redisCtx, cancelRedis := context.WithTimeout(context.Background(), 10*time.Second)
	redisClient, err := database.ConnectRedis(redisCtx, cfg.redisURL)
	cancelRedis()
	if err != nil {
		slog.Error("identity Redis connection failed", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	tokens := coreauth.NewTokenService(cfg.jwtSecret)
	authService := identityauth.NewService(db, tokens, redisClient)
	authService.ConfigurePasswordReset(mailer.NewSMTP(cfg.smtpHost, cfg.smtpPort, cfg.smtpUser, cfg.smtpPassword, cfg.smtpFrom), cfg.passwordResetURL)
	userService := user.New(shared.New(db))
	ready := func(ctx context.Context) error {
		if err := database.Ping(ctx, db); err != nil {
			return err
		}
		if err := redisClient.Ping(ctx).Err(); err != nil {
			return err
		}
		var tablesReady bool
		if err := db.WithContext(ctx).Raw("SELECT to_regclass('public.users') IS NOT NULL AND to_regclass('public.shipping_addresses') IS NOT NULL AND to_regclass('public.roles') IS NOT NULL").Scan(&tablesReady).Error; err != nil {
			return err
		}
		if !tablesReady {
			return errors.New("identity tables unavailable")
		}
		return nil
	}
	httpRouter := identityapi.NewRouter(authService, userService, tokens, ready)
	httpServer := &http.Server{Addr: ":" + cfg.httpPort, Handler: httpRouter, ReadHeaderTimeout: 5 * time.Second}

	listener, err := net.Listen("tcp", ":"+cfg.grpcPort)
	if err != nil {
		slog.Error("identity gRPC listener failed", "error", err)
		os.Exit(1)
	}
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(grpcutil.UnaryAuth(tokens)))
	identityv1.RegisterIdentityServiceServer(grpcServer, grpcidentity.NewServer(authService))

	errorsCh := make(chan error, 2)
	go func() {
		slog.Info("identity HTTP server listening", "port", cfg.httpPort)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorsCh <- fmt.Errorf("identity HTTP server: %w", err)
		}
	}()
	go func() {
		slog.Info("identity gRPC server listening", "port", cfg.grpcPort)
		if err := grpcServer.Serve(listener); err != nil {
			errorsCh <- fmt.Errorf("identity gRPC server: %w", err)
		}
	}()

	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	select {
	case <-signalCtx.Done():
	case err := <-errorsCh:
		slog.Error("identity service failed", "error", err)
	}
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("identity HTTP shutdown failed", "error", err)
	}
	grpcStopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(grpcStopped)
	}()
	select {
	case <-grpcStopped:
	case <-shutdownCtx.Done():
		grpcServer.Stop()
		slog.Error("identity gRPC shutdown timed out", "error", shutdownCtx.Err())
	}
}

func loadConfig() (config, error) {
	cfg := config{
		databaseURL:      os.Getenv("DATABASE_URL"),
		redisURL:         os.Getenv("REDIS_URL"),
		jwtSecret:        os.Getenv("JWT_SECRET"),
		httpPort:         value("IDENTITY_HTTP_PORT", "8084"),
		grpcPort:         value("IDENTITY_GRPC_PORT", "9092"),
		smtpHost:         os.Getenv("SMTP_HOST"),
		smtpPort:         intValue("SMTP_PORT", 587),
		smtpUser:         os.Getenv("SMTP_USER"),
		smtpPassword:     os.Getenv("SMTP_PASSWORD"),
		smtpFrom:         os.Getenv("SMTP_FROM"),
		passwordResetURL: os.Getenv("PASSWORD_RESET_URL"),
	}
	if cfg.databaseURL == "" || cfg.redisURL == "" || cfg.jwtSecret == "" {
		return config{}, errors.New("DATABASE_URL, REDIS_URL and JWT_SECRET are required")
	}
	if err := validPort("IDENTITY_HTTP_PORT", cfg.httpPort); err != nil {
		return config{}, err
	}
	if err := validPort("IDENTITY_GRPC_PORT", cfg.grpcPort); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func validPort(name, raw string) error {
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("%s must be between 1 and 65535", name)
	}
	return nil
}

func intValue(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 || parsed > 65535 {
		return fallback
	}
	return parsed
}

func value(key, fallback string) string {
	if result := os.Getenv(key); result != "" {
		return result
	}
	return fallback
}
