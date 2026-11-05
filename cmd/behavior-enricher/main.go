package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/example/e-commerce-be/internal/database"
	"github.com/example/e-commerce-be/internal/modules/recommendation"
	"github.com/example/e-commerce-be/internal/productsearch"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	brokers := os.Getenv("KAFKA_BROKERS")
	if len(recommendation.BrokerAddresses(brokers)) == 0 || os.Getenv("DATABASE_URL") == "" || os.Getenv("REDIS_URL") == "" {
		slog.Error("behavior enricher configuration missing")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	db, err := database.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	redisClient, err := database.ConnectRedis(ctx, os.Getenv("REDIS_URL"))
	if err != nil {
		slog.Error("Redis unavailable", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()
	es, err := database.ConnectElasticsearch(ctx, os.Getenv("ELASTICSEARCH_URL"), "elasticsearch")
	if err != nil {
		slog.Warn("Elasticsearch unavailable; search events will be retried", "error", err)
	}
	if es != nil {
		defer es.Close()
	}
	var search *productsearch.Client
	if es != nil {
		search = productsearch.New(es)
	}
	reader := recommendation.NewReader(brokers, recommendation.RawTopic, "behavior-enricher-v1")
	defer reader.Close()
	writer := recommendation.NewWriter(brokers, recommendation.EnrichedTopic)
	defer writer.Close()
	dead := recommendation.NewWriter(brokers, recommendation.DeadTopic)
	defer dead.Close()
	worker := &recommendation.Enricher{Repo: recommendation.NewRepository(db), Redis: redisClient, Search: search, Reader: reader, Writer: writer, DeadWriter: dead}
	for ctx.Err() == nil {
		if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
			slog.Error("behavior enricher restarted", "error", err)
			time.Sleep(time.Second)
		}
	}
}
