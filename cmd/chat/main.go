package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/database"
	chatv1 "github.com/example/e-commerce-be/internal/gen/chat/v1"
	"github.com/example/e-commerce-be/internal/grpcchat"
	"github.com/example/e-commerce-be/internal/grpcidentity"
	"github.com/example/e-commerce-be/internal/grpcseller"
	"github.com/example/e-commerce-be/internal/grpcutil"
	"github.com/example/e-commerce-be/internal/http/chatserver"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
)

type config struct {
	databaseURL    string
	redisURL       string
	jwtSecret      string
	httpPort       string
	grpcPort       string
	identityTarget string
	sellerTarget   string
	origins        []string
}

func main() {
	_ = godotenv.Load()
	cfg, err := loadConfig()
	if err != nil {
		slog.Error("invalid chat configuration", "error", err)
		os.Exit(1)
	}
	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	db, err := database.Connect(cfg.databaseURL)
	if err != nil {
		slog.Error("chat database connection failed", "error", err)
		os.Exit(1)
	}
	for {
		err = database.MigrateChat(db)
		if err == nil {
			break
		}
		if !errors.Is(err, database.ErrChatBootstrapPending) {
			slog.Error("chat migration failed", "error", err)
			os.Exit(1)
		}
		select {
		case <-signalCtx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
	redisCtx, cancelRedis := context.WithTimeout(context.Background(), 10*time.Second)
	redisClient, err := database.ConnectRedis(redisCtx, cfg.redisURL)
	cancelRedis()
	if err != nil {
		slog.Error("chat Redis connection failed", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()
	identityConnection, identityClient, err := grpcidentity.Dial(cfg.identityTarget)
	if err != nil {
		slog.Error("identity gRPC client initialization failed", "error", err)
		os.Exit(1)
	}
	defer identityConnection.Close()
	sellerConnection, sellerClient, err := grpcseller.Dial(cfg.sellerTarget)
	if err != nil {
		slog.Error("seller gRPC client initialization failed", "error", err)
		os.Exit(1)
	}
	defer sellerConnection.Close()

	tokens := coreauth.NewTokenService(cfg.jwtSecret)
	runtime := chatserver.New(db, tokens, redisClient, cfg.origins, identityClient, sellerClient.WithServiceIdentity(tokens, "chat"))
	defer runtime.Close()

	listener, err := net.Listen("tcp", ":"+cfg.grpcPort)
	if err != nil {
		slog.Error("chat gRPC listener failed", "error", err)
		os.Exit(1)
	}
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(grpcutil.UnaryAuth(tokens)))
	chatv1.RegisterChatServiceServer(grpcServer, grpcchat.NewServer(runtime.Service))
	httpServer := &http.Server{Addr: ":" + cfg.httpPort, Handler: runtime.Router, ReadHeaderTimeout: 5 * time.Second}

	errorsCh := make(chan error, 2)
	go func() {
		slog.Info("chat HTTP/WebSocket server listening", "port", cfg.httpPort)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorsCh <- fmt.Errorf("chat HTTP server: %w", err)
		}
	}()
	go func() {
		slog.Info("chat gRPC server listening", "port", cfg.grpcPort)
		if err := grpcServer.Serve(listener); err != nil {
			errorsCh <- fmt.Errorf("chat gRPC server: %w", err)
		}
	}()

	select {
	case <-signalCtx.Done():
	case err := <-errorsCh:
		slog.Error("chat service failed", "error", err)
	}

	runtime.Close()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("chat HTTP shutdown failed", "error", err)
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
		slog.Error("chat gRPC shutdown timed out", "error", shutdownCtx.Err())
	}
}

func loadConfig() (config, error) {
	cfg := config{
		databaseURL:    os.Getenv("DATABASE_URL"),
		redisURL:       os.Getenv("REDIS_URL"),
		jwtSecret:      os.Getenv("JWT_SECRET"),
		httpPort:       value("CHAT_HTTP_PORT", "8083"),
		grpcPort:       value("CHAT_GRPC_PORT", "9091"),
		identityTarget: value("IDENTITY_GRPC_TARGET", "identity:9092"),
		sellerTarget:   value("SELLER_GRPC_TARGET", "seller:9093"),
	}
	if cfg.databaseURL == "" || cfg.redisURL == "" || cfg.jwtSecret == "" {
		return config{}, errors.New("DATABASE_URL, REDIS_URL and JWT_SECRET are required")
	}
	if err := validPort("CHAT_HTTP_PORT", cfg.httpPort); err != nil {
		return config{}, err
	}
	if err := validPort("CHAT_GRPC_PORT", cfg.grpcPort); err != nil {
		return config{}, err
	}
	if strings.TrimSpace(cfg.identityTarget) == "" {
		return config{}, errors.New("IDENTITY_GRPC_TARGET is required")
	}
	if strings.TrimSpace(cfg.sellerTarget) == "" {
		return config{}, errors.New("SELLER_GRPC_TARGET is required")
	}
	for _, raw := range strings.Split(os.Getenv("WEBSOCKET_ORIGINS"), ",") {
		origin := strings.TrimSpace(raw)
		if origin == "" {
			continue
		}
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return config{}, errors.New("WEBSOCKET_ORIGINS must contain exact HTTP(S) origins without paths")
		}
		cfg.origins = append(cfg.origins, origin)
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

func value(key, fallback string) string {
	if result := os.Getenv(key); result != "" {
		return result
	}
	return fallback
}
