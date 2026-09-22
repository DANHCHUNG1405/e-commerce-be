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
	sellerv1 "github.com/example/e-commerce-be/internal/gen/seller/v1"
	"github.com/example/e-commerce-be/internal/grpcseller"
	"github.com/example/e-commerce-be/internal/http/sellerapi"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
)

func main() {
	_ = godotenv.Load()
	databaseURL, secret := os.Getenv("DATABASE_URL"), os.Getenv("JWT_SECRET")
	httpPort, grpcPort := port("SELLER_HTTP_PORT", "8085"), port("SELLER_GRPC_PORT", "9093")
	if databaseURL == "" || secret == "" || !validPort(httpPort) || !validPort(grpcPort) {
		slog.Error("invalid seller configuration: DATABASE_URL, JWT_SECRET, SELLER_HTTP_PORT and SELLER_GRPC_PORT are required")
		os.Exit(1)
	}
	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	db, err := database.Connect(databaseURL)
	if err != nil {
		slog.Error("seller database connection failed", "error", err)
		os.Exit(1)
	}
	for {
		err = database.MigrateSeller(db)
		if err == nil {
			break
		}
		if !errors.Is(err, database.ErrSellerBootstrapPending) {
			slog.Error("seller migration failed", "error", err)
			os.Exit(1)
		}
		select {
		case <-signalCtx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
	tokens := coreauth.NewTokenService(secret)
	ready := func(ctx context.Context) error {
		if err := database.Ping(ctx, db); err != nil {
			return err
		}
		var tablesReady bool
		if err := db.WithContext(ctx).Raw("SELECT to_regclass('seller.sellers') IS NOT NULL AND to_regclass('seller.seller_members') IS NOT NULL").Scan(&tablesReady).Error; err != nil {
			return err
		}
		if !tablesReady {
			return errors.New("seller tables unavailable")
		}
		return nil
	}
	httpServer := &http.Server{Addr: ":" + httpPort, Handler: sellerapi.NewRouter(db, tokens, ready), ReadHeaderTimeout: 5 * time.Second}
	listener, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		slog.Error("seller gRPC listener failed", "error", err)
		os.Exit(1)
	}
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(grpcseller.UnaryAuth(tokens)))
	sellerv1.RegisterSellerServiceServer(grpcServer, grpcseller.NewServer(db))
	errorsCh := make(chan error, 2)
	go func() {
		slog.Info("seller HTTP server listening", "port", httpPort)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorsCh <- fmt.Errorf("seller HTTP server: %w", err)
		}
	}()
	go func() {
		slog.Info("seller gRPC server listening", "port", grpcPort)
		if err := grpcServer.Serve(listener); err != nil {
			errorsCh <- fmt.Errorf("seller gRPC server: %w", err)
		}
	}()
	select {
	case <-signalCtx.Done():
	case err := <-errorsCh:
		slog.Error("seller service failed", "error", err)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("seller HTTP shutdown failed", "error", err)
	}
	stopped := make(chan struct{})
	go func() { grpcServer.GracefulStop(); close(stopped) }()
	select {
	case <-stopped:
	case <-shutdownCtx.Done():
		grpcServer.Stop()
	}
}

func port(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
func validPort(raw string) bool {
	value, err := strconv.Atoi(raw)
	return err == nil && value >= 1 && value <= 65535
}
