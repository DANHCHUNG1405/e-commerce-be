package database

import (
	"embed"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

//go:embed migrations/assistant/*.sql
var assistantMigrations embed.FS

func MigrateAssistant(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`CREATE SCHEMA IF NOT EXISTS assistant`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`CREATE TABLE IF NOT EXISTS assistant.schema_migrations (version BIGINT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(682461954)`).Error; err != nil {
			return err
		}
		entries, err := assistantMigrations.ReadDir("migrations/assistant")
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
				return fmt.Errorf("assistant migration %s: invalid version: %w", entry.Name(), err)
			}
			var applied bool
			if err := tx.Raw(`SELECT EXISTS (SELECT 1 FROM assistant.schema_migrations WHERE version=?)`, version).Scan(&applied).Error; err != nil {
				return err
			}
			if applied {
				continue
			}
			sql, err := assistantMigrations.ReadFile("migrations/assistant/" + entry.Name())
			if err != nil {
				return err
			}
			if err := tx.Exec(string(sql)).Error; err != nil {
				return fmt.Errorf("assistant migration %s: %w", entry.Name(), err)
			}
			if err := tx.Exec(`INSERT INTO assistant.schema_migrations(version) VALUES (?)`, version).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

var ErrAssistantSchemaUnavailable = errors.New("assistant schema unavailable")
