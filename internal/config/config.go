package config

import (
	"errors"
	"fmt"
	"github.com/example/e-commerce-be/internal/modules/payment"
	"net/url"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	WebSocketOrigins []string
	SePay            payment.Config
	Environment      string
	Port             string
	DatabaseURL      string
	MongoURL         string
	MongoDatabase    string
	RedisURL         string
	JWTSecret        string
	SMTPHost         string
	SMTPPort         int
	SMTPUser         string
	SMTPPassword     string
	SMTPFrom         string
	PasswordResetURL string
}

func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		SePay:            payment.Config{Bank: os.Getenv("SEPAY_BANK"), Account: os.Getenv("SEPAY_ACCOUNT_NUMBER"), WebhookSecret: os.Getenv("SEPAY_WEBHOOK_SECRET")},
		Environment:      valueOrDefault("APP_ENV", "development"),
		Port:             valueOrDefault("PORT", "8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		MongoURL:         os.Getenv("MONGO_URL"),
		MongoDatabase:    valueOrDefault("MONGO_DATABASE", "ecommerce"),
		RedisURL:         os.Getenv("REDIS_URL"),
		JWTSecret:        os.Getenv("JWT_SECRET"),
		SMTPHost:         os.Getenv("SMTP_HOST"),
		SMTPPort:         intValueOrDefault("SMTP_PORT", 587),
		SMTPUser:         os.Getenv("SMTP_USER"),
		SMTPPassword:     os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:         os.Getenv("SMTP_FROM"),
		PasswordResetURL: os.Getenv("PASSWORD_RESET_URL"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	for _, raw := range strings.Split(os.Getenv("WEBSOCKET_ORIGINS"), ",") {
		origin := strings.TrimSpace(raw)
		if origin == "" {
			continue
		}
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return Config{}, errors.New("WEBSOCKET_ORIGINS must contain exact HTTP(S) origins without paths")
		}
		cfg.WebSocketOrigins = append(cfg.WebSocketOrigins, origin)
	}
	if (cfg.SePay.Bank != "" || cfg.SePay.Account != "" || cfg.SePay.WebhookSecret != "") && !cfg.SePay.Enabled() {
		return Config{}, errors.New("SePay requires SEPAY_BANK, SEPAY_ACCOUNT_NUMBER and SEPAY_WEBHOOK_SECRET (at least 32 characters)")
	}
	if cfg.MongoURL == "" {
		return Config{}, errors.New("MONGO_URL is required")
	}
	if cfg.RedisURL == "" {
		return Config{}, errors.New("REDIS_URL is required")
	}
	if cfg.JWTSecret == "" {
		return Config{}, errors.New("JWT_SECRET is required")
	}
	return cfg, nil
}

func valueOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func intValueOrDefault(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil || parsed < 1 || parsed > 65535 {
		return fallback
	}
	return parsed
}
