package database

import (
	"embed"
	"gorm.io/gorm"
)

//go:embed migrations/notification/*.sql
var notificationMigrations embed.FS

func MigrateNotifications(db *gorm.DB) error {
	return db.Exec(string(mustReadNotificationMigration())).Error
}

func mustReadNotificationMigration() []byte {
	b, err := notificationMigrations.ReadFile("migrations/notification/0001_notifications.sql")
	if err != nil {
		panic(err)
	}
	return b
}
