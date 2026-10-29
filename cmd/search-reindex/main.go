package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/example/e-commerce-be/internal/database"
	"github.com/example/e-commerce-be/internal/events"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/catalog"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/example/e-commerce-be/internal/productsearch"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
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
	ctx := context.Background()
	es, err := database.ConnectElasticsearch(ctx, value("ELASTICSEARCH_URL", "http://localhost:9200"))
	if err != nil {
		slog.Error("invalid Elasticsearch configuration", "error", err)
		os.Exit(1)
	}
	defer es.Close()
	client := productsearch.New(es)
	index := fmt.Sprintf("products-%s-%s", time.Now().UTC().Format("20060102t150405"), uuid.NewString()[:8])
	snapshotStarted := time.Now().UTC()
	if err := client.EnsureIndex(ctx, index); err != nil {
		slog.Error("create versioned product index failed", "error", err)
		os.Exit(1)
	}
	service := catalog.NewWithSearch(shared.New(db), es, "elasticsearch")
	var cursor uuid.UUID
	count := 0
	for {
		var ids []uuid.UUID
		q := db.WithContext(ctx).Model(&models.Product{}).Where("deleted_at IS NULL").Order("id").Limit(500)
		if cursor != uuid.Nil {
			q = q.Where("id > ?", cursor)
		}
		if err := q.Pluck("id", &ids).Error; err != nil {
			slog.Error("read PostgreSQL product snapshot failed", "error", err)
			os.Exit(1)
		}
		if len(ids) == 0 {
			break
		}
		for _, id := range ids {
			doc, ok, err := service.SearchDocument(ctx, id)
			if err != nil {
				slog.Error("build search document failed", "productId", id, "error", err)
				os.Exit(1)
			}
			if ok {
				if err := client.IndexTo(ctx, index, doc); err != nil {
					slog.Error("index product failed", "productId", id, "error", err)
					os.Exit(1)
				}
				count++
			}
		}
		cursor = ids[len(ids)-1]
	}
	if err := client.SetAlias(ctx, index); err != nil {
		slog.Error("switch product search alias failed", "error", err)
		os.Exit(1)
	}
	if err := replaySearchEvents(ctx, db, service, snapshotStarted); err != nil {
		slog.Error("replay product changes after reindex failed", "error", err)
		os.Exit(1)
	}
	slog.Info("product search reindex completed", "index", index, "documents", count)
}

func replaySearchEvents(ctx context.Context, db *gorm.DB, service *catalog.Service, since time.Time) error {
	var rows []models.OutboxEvent
	types := []string{events.ProductSearchUpdated, events.InventoryChanged, events.ReviewSearchUpdated, events.SellerStatusChanged, events.SellerSearchUpdated, events.OrderStatusChanged}
	if err := db.WithContext(ctx).Where("created_at >= ? AND event_type IN ?", since, types).Order("created_at,id").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		ids, err := affectedProducts(ctx, db, row)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := service.IndexProduct(ctx, id); err != nil {
				return err
			}
		}
	}
	return nil
}

func affectedProducts(ctx context.Context, db *gorm.DB, row models.OutboxEvent) ([]uuid.UUID, error) {
	switch row.EventType {
	case events.ProductSearchUpdated, events.ReviewSearchUpdated:
		if row.AggregateType != "product" {
			return nil, nil
		}
		return []uuid.UUID{row.AggregateID}, nil
	case events.InventoryChanged:
		var id uuid.UUID
		err := db.WithContext(ctx).Model(&models.ProductVariant{}).Where("id=?", row.AggregateID).Pluck("product_id", &id).Error
		if err != nil {
			return nil, err
		}
		if id == uuid.Nil {
			return nil, nil
		}
		return []uuid.UUID{id}, nil
	case events.SellerStatusChanged, events.SellerSearchUpdated:
		ids := []uuid.UUID{}
		err := db.WithContext(ctx).Model(&models.Product{}).Where("seller_id=?", row.AggregateID).Pluck("id", &ids).Error
		return ids, err
	case events.OrderStatusChanged:
		var payload events.OrderStatusChangedPayload
		if json.Unmarshal(row.Payload, &payload) != nil || payload.Status != "delivered" || payload.SellerOrderID == nil {
			return nil, nil
		}
		ids := []uuid.UUID{}
		err := db.WithContext(ctx).Table("order_items i").Joins("JOIN product_variants v ON v.id=i.variant_id").Where("i.seller_order_id=?", *payload.SellerOrderID).Distinct("v.product_id").Pluck("v.product_id", &ids).Error
		return ids, err
	default:
		return nil, nil
	}
}

func value(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
