package database

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

//go:embed migrations/seller/*.sql
var sellerMigrations embed.FS

var ErrSellerBootstrapPending = errors.New("seller bootstrap migration is pending")

func WaitSellerSchema(ctx context.Context, db *gorm.DB) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		var ready bool
		err := db.WithContext(ctx).Raw("SELECT to_regclass('seller.sellers') IS NOT NULL AND to_regclass('seller.seller_members') IS NOT NULL").Scan(&ready).Error
		if err != nil {
			return fmt.Errorf("check seller schema: %w", err)
		}
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for seller schema: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func MigrateSeller(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("CREATE SCHEMA IF NOT EXISTS seller").Error; err != nil {
			return err
		}
		if err := tx.Exec("CREATE TABLE IF NOT EXISTS seller.schema_migrations (version BIGINT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())").Error; err != nil {
			return err
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(682461955)").Error; err != nil {
			return err
		}
		entries, err := sellerMigrations.ReadDir("migrations/seller")
		if err != nil {
			return err
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			version, err := strconv.ParseInt(strings.SplitN(entry.Name(), "_", 2)[0], 10, 64)
			if err != nil {
				return fmt.Errorf("seller migration %s: invalid version: %w", entry.Name(), err)
			}
			var applied int64
			if err := tx.Raw("SELECT COUNT(*) FROM seller.schema_migrations WHERE version=?", version).Scan(&applied).Error; err != nil {
				return err
			}
			if applied > 0 {
				continue
			}
			if version == 1 {
				var coreTable bool
				if err := tx.Raw("SELECT to_regclass('public.schema_migrations') IS NOT NULL").Scan(&coreTable).Error; err != nil {
					return err
				}
				if !coreTable {
					return ErrSellerBootstrapPending
				}
				var coreReady int64
				if err := tx.Raw("SELECT COUNT(*) FROM public.schema_migrations WHERE version=10").Scan(&coreReady).Error; err != nil {
					return err
				}
				var tablesReady bool
				if err := tx.Raw("SELECT to_regclass('public.sellers') IS NOT NULL AND to_regclass('public.seller_members') IS NOT NULL").Scan(&tablesReady).Error; err != nil {
					return err
				}
				if coreReady == 0 || !tablesReady {
					return ErrSellerBootstrapPending
				}
			}
			sql, err := sellerMigrations.ReadFile("migrations/seller/" + entry.Name())
			if err != nil {
				return err
			}
			if err := tx.Exec(string(sql)).Error; err != nil {
				return fmt.Errorf("seller migration %s: %w", entry.Name(), err)
			}
			if err := tx.Exec("INSERT INTO seller.schema_migrations(version) VALUES (?)", version).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
