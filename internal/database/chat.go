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

//go:embed migrations/chat/*.sql
var chatMigrations embed.FS

// ErrChatBootstrapPending means the core migration has not created the legacy
// public chat tables yet. Chat startup can retry while the API bootstraps a new DB.
var ErrChatBootstrapPending = errors.New("chat bootstrap migration is pending")

func MigrateChat(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`CREATE SCHEMA IF NOT EXISTS chat`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`CREATE TABLE IF NOT EXISTS chat.schema_migrations (version BIGINT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(682461953)`).Error; err != nil {
			return err
		}
		entries, err := chatMigrations.ReadDir("migrations/chat")
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
				return fmt.Errorf("chat migration %s: invalid version: %w", entry.Name(), err)
			}
			var count int64
			if err := tx.Raw(`SELECT COUNT(*) FROM chat.schema_migrations WHERE version=?`, version).Scan(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				continue
			}
			if version == 1 {
				var ready bool
				if err := tx.Raw(`SELECT to_regclass('public.chat_conversations') IS NOT NULL AND to_regclass('public.chat_messages') IS NOT NULL AND to_regclass('public.chat_reads') IS NOT NULL`).Scan(&ready).Error; err != nil {
					return err
				}
				if !ready {
					return ErrChatBootstrapPending
				}
			}
			sql, err := chatMigrations.ReadFile("migrations/chat/" + entry.Name())
			if err != nil {
				return err
			}
			if err := tx.Exec(string(sql)).Error; err != nil {
				return fmt.Errorf("chat migration %s: %w", entry.Name(), err)
			}
			if err := tx.Exec(`INSERT INTO chat.schema_migrations(version) VALUES (?)`, version).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
