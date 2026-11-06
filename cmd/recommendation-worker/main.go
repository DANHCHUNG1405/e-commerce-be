package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
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
		slog.Error("recommendation worker configuration missing")
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

	esURL := os.Getenv("ELASTICSEARCH_URL")
	if esURL == "" {
		esURL = "http://localhost:9200"
	}
	es, err := database.ConnectElasticsearch(ctx, esURL, "elasticsearch")
	if err != nil {
		slog.Error("Elasticsearch configuration invalid", "error", err)
		os.Exit(1)
	}
	defer es.Close()
	search := productsearch.New(es)

	clickHouseURL := os.Getenv("CLICKHOUSE_URL")
	if clickHouseURL == "" {
		clickHouseURL = "http://localhost:8123"
	}
	store := &recommendation.ClickHouse{URL: clickHouseURL, User: os.Getenv("CLICKHOUSE_USER"), Password: os.Getenv("CLICKHOUSE_PASSWORD")}

	rawReader := recommendation.NewReader(brokers, recommendation.RawTopic, "behavior-enricher-v1")
	enrichedProfileReader := recommendation.NewReader(brokers, recommendation.EnrichedTopic, "recommendation-profile-v1")
	enrichedSinkReader := recommendation.NewReader(brokers, recommendation.EnrichedTopic, "behavior-clickhouse-v1")
	defer rawReader.Close()
	defer enrichedProfileReader.Close()
	defer enrichedSinkReader.Close()
	enrichedWriter := recommendation.NewWriter(brokers, recommendation.EnrichedTopic)
	enricherDeadWriter := recommendation.NewWriter(brokers, recommendation.DeadTopic)
	profileDeadWriter := recommendation.NewWriter(brokers, recommendation.DeadTopic)
	sinkDeadWriter := recommendation.NewWriter(brokers, recommendation.DeadTopic)
	defer enrichedWriter.Close()
	defer enricherDeadWriter.Close()
	defer profileDeadWriter.Close()
	defer sinkDeadWriter.Close()

	enricher := &recommendation.Enricher{Repo: recommendation.NewRepository(db), Redis: redisClient, Search: search, Reader: rawReader, Writer: enrichedWriter, DeadWriter: enricherDeadWriter}
	projector := &recommendation.ProfileWorker{Reader: enrichedProfileReader, Redis: redisClient, DeadWriter: profileDeadWriter}
	sink := &recommendation.ClickHouseSink{Reader: enrichedSinkReader, Store: store, DeadWriter: sinkDeadWriter}

	workers := []struct {
		name string
		run  func(context.Context) error
	}{
		{name: "behavior-enricher", run: enricher.Run},
		{name: "recommendation-projector", run: projector.Run},
		{name: "behavior-sink", run: func(workerCtx context.Context) error {
			for {
				if err := store.Migrate(workerCtx); err == nil {
					break
				} else {
					slog.Warn("ClickHouse migration retry", "error", err)
					select {
					case <-workerCtx.Done():
						return workerCtx.Err()
					case <-time.After(5 * time.Second):
					}
				}
			}
			return sink.Run(workerCtx)
		}},
	}
	var group sync.WaitGroup
	for _, worker := range workers {
		worker := worker
		group.Add(1)
		go func() { defer group.Done(); supervise(ctx, worker.name, worker.run) }()
	}
	slog.Info("recommendation workers started", "consumerGroups", len(workers))
	<-ctx.Done()
	group.Wait()
}

func supervise(ctx context.Context, name string, run func(context.Context) error) {
	for ctx.Err() == nil {
		if err := run(ctx); ctx.Err() != nil {
			return
		} else {
			slog.Error("recommendation worker restarting", "worker", name, "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}
}
