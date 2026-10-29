package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/example/e-commerce-be/internal/database"
	"github.com/example/e-commerce-be/internal/events"
	"github.com/example/e-commerce-be/internal/kafka"
	"github.com/example/e-commerce-be/internal/modules/catalog"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/example/e-commerce-be/internal/searchindexer"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	db, err := database.Connect(dbURL)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	es, err := database.ConnectElasticsearch(ctx, value("ELASTICSEARCH_URL", "http://elasticsearch:9200"))
	if err != nil {
		slog.Error("invalid Elasticsearch configuration", "error", err)
		os.Exit(1)
	}
	defer es.Close()
	brokers := value("KAFKA_BROKERS", "kafka:9092")
	if err := kafka.Wait(ctx, brokers); err != nil {
		return
	}
	reader := kafka.NewReader(brokers, events.Topic, "search-indexer")
	defer reader.Close()
	service := catalog.NewWithSearch(shared.New(db), es, "elasticsearch")
	slog.Info("search indexer started", "group", "search-indexer", "topic", events.Topic)
	if err := searchindexer.New(db, reader, service).Run(ctx); err != nil {
		slog.Error("search indexer stopped", "error", err)
		os.Exit(1)
	}
}
func value(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
