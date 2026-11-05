package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/database"
	"github.com/example/e-commerce-be/internal/grpcseller"
	"github.com/example/e-commerce-be/internal/http/assistantapi"
	"github.com/example/e-commerce-be/internal/modules/assistant"
	"github.com/example/e-commerce-be/internal/modules/catalog"
	"github.com/example/e-commerce-be/internal/modules/order"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	port := value("ASSISTANT_PORT", "8086")
	databaseURL := os.Getenv("DATABASE_URL")
	redisURL := os.Getenv("REDIS_URL")
	jwtSecret := os.Getenv("JWT_SECRET")
	if databaseURL == "" || redisURL == "" || jwtSecret == "" || os.Getenv("GEMINI_API_KEY") == "" {
		slog.Error("DATABASE_URL, REDIS_URL, JWT_SECRET and GEMINI_API_KEY are required")
		os.Exit(1)
	}
	db, err := database.Connect(databaseURL)
	if err != nil {
		slog.Error("assistant database connection failed", "error", err)
		os.Exit(1)
	}
	if err := database.MigrateAssistant(db); err != nil {
		slog.Error("assistant migration failed", "error", err)
		os.Exit(1)
	}
	sellerConnection, sellerClient, err := grpcseller.Dial(value("SELLER_GRPC_TARGET", "seller:9093"))
	if err != nil {
		slog.Error("seller gRPC client initialization failed", "error", err)
		os.Exit(1)
	}
	defer sellerConnection.Close()
	redisCtx, cancelRedis := context.WithTimeout(context.Background(), 10*time.Second)
	redisClient, err := database.ConnectRedis(redisCtx, redisURL)
	cancelRedis()
	if err != nil {
		slog.Error("assistant Redis connection failed", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()
	modelCtx, cancelModel := context.WithTimeout(context.Background(), 10*time.Second)
	lookup := &supportLookup{catalog: catalog.New(shared.New(db)), orders: order.New(order.NewRepository(db)), sellers: sellerClient, policies: assistant.NewPolicyRepository(db)}
	model, err := assistant.NewGemini(modelCtx, os.Getenv("GEMINI_API_KEY"), value("GEMINI_MODEL", "gemini-3.8-flash"), lookup)
	cancelModel()
	if err != nil {
		slog.Error("Gemini client initialization failed", "error", err)
		os.Exit(1)
	}
	service := assistant.NewService(assistant.NewRepository(db), model, assistant.NewRedisLimiter(redisClient))
	router := assistantapi.NewRouter(coreauth.NewTokenService(jwtSecret), service, func(ctx context.Context) error {
		if err := database.Ping(ctx, db); err != nil {
			return err
		}
		return redisClient.Ping(ctx).Err()
	})
	server := &http.Server{Addr: ":" + port, Handler: router, ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		slog.Info("assistant service listening", "port", port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("assistant service failed", "error", err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("assistant shutdown failed", "error", err)
	}
}
func value(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
