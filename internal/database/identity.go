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

// WaitIdentitySchema keeps the API from serving requests while Identity moves
// legacy tables after core bootstrap on a new database.
func WaitIdentitySchema(ctx context.Context, db *gorm.DB) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		var ready bool
		err := db.WithContext(ctx).Raw(`SELECT to_regclass('identity.users') IS NOT NULL AND to_regclass('identity.roles') IS NOT NULL AND to_regclass('identity.user_roles') IS NOT NULL AND to_regclass('identity.shipping_addresses') IS NOT NULL`).Scan(&ready).Error
		if err != nil {
			return fmt.Errorf("check identity schema: %w", err)
		}
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for identity schema: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

//go:embed migrations/identity/*.sql
var identityMigrations embed.FS

// ErrIdentityBootstrapPending means the API has not finished its legacy core
// migrations. Identity startup retries without changing those applied files.
var ErrIdentityBootstrapPending = errors.New("identity bootstrap migration is pending")

func MigrateIdentity(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`CREATE SCHEMA IF NOT EXISTS identity`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`CREATE TABLE IF NOT EXISTS identity.schema_migrations (version BIGINT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(682461954)`).Error; err != nil {
			return err
		}
		entries, err := identityMigrations.ReadDir("migrations/identity")
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
				return fmt.Errorf("identity migration %s: invalid version: %w", entry.Name(), err)
			}
			var count int64
			if err := tx.Raw(`SELECT COUNT(*) FROM identity.schema_migrations WHERE version=?`, version).Scan(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				continue
			}
			if version == 1 {
				var coreTable bool
				if err := tx.Raw(`SELECT to_regclass('public.schema_migrations') IS NOT NULL`).Scan(&coreTable).Error; err != nil {
					return err
				}
				if !coreTable {
					return ErrIdentityBootstrapPending
				}
				var coreReady int64
				if err := tx.Raw(`SELECT COUNT(*) FROM public.schema_migrations WHERE version=9`).Scan(&coreReady).Error; err != nil {
					return err
				}
				var tablesReady bool
				if err := tx.Raw(`SELECT to_regclass('public.users') IS NOT NULL AND to_regclass('public.roles') IS NOT NULL AND to_regclass('public.user_roles') IS NOT NULL AND to_regclass('public.shipping_addresses') IS NOT NULL`).Scan(&tablesReady).Error; err != nil {
					return err
				}
				if coreReady == 0 || !tablesReady {
					return ErrIdentityBootstrapPending
				}
			}
			sql, err := identityMigrations.ReadFile("migrations/identity/" + entry.Name())
			if err != nil {
				return err
			}
			if err := tx.Exec(string(sql)).Error; err != nil {
				return fmt.Errorf("identity migration %s: %w", entry.Name(), err)
			}
			if err := tx.Exec(`INSERT INTO identity.schema_migrations(version) VALUES (?)`, version).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
