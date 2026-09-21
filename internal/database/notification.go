package database

import (
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

//go:embed migrations/notification/*.sql
var notificationMigrations embed.FS

func MigrateNotifications(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`CREATE SCHEMA IF NOT EXISTS notification`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`CREATE TABLE IF NOT EXISTS notification.schema_migrations (version BIGINT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(682461952)`).Error; err != nil {
			return err
		}
		entries, err := notificationMigrations.ReadDir("migrations/notification")
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
				return fmt.Errorf("notification migration %s: invalid version: %w", entry.Name(), err)
			}
			var count int64
			if err := tx.Raw(`SELECT COUNT(*) FROM notification.schema_migrations WHERE version=?`, version).Scan(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				continue
			}
			sql, err := notificationMigrations.ReadFile("migrations/notification/" + entry.Name())
			if err != nil {
				return err
			}
			if err := tx.Exec(string(sql)).Error; err != nil {
				return fmt.Errorf("notification migration %s: %w", entry.Name(), err)
			}
			if err := tx.Exec(`INSERT INTO notification.schema_migrations(version) VALUES (?)`, version).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
