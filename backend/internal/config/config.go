// Package config — конфигурация backend из переменных окружения.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Dev-дефолты совпадают с deploy/dev/.env.example; это не секреты, а локальная dev-сборка.
const (
	defaultDatabaseURL  = "postgres://coffeely:coffeely@localhost:5432/coffeely?sslmode=disable"
	defaultJWTDevSecret = "dev-insecure-secret"
)

// Config — все настройки сервера, читаемые из env.
type Config struct {
	Port        string // PORT
	DatabaseURL string // DATABASE_URL
	Env         string // ENV: dev / prod

	JWTSecret       string        // JWT_SECRET (в prod обязателен)
	AccessTokenTTL  time.Duration // ACCESS_TOKEN_TTL (15m)
	RefreshTokenTTL time.Duration // REFRESH_TOKEN_TTL (720h = 30 дней)

	// Yandex ID (OAuth2). Auth/Token/Info URL переопределяются в тестах (mock OIDC).
	YandexClientID     string // YANDEX_CLIENT_ID
	YandexClientSecret string // YANDEX_CLIENT_SECRET
	YandexRedirectURL  string // YANDEX_REDIRECT_URL
	YandexAuthURL      string // YANDEX_AUTH_URL
	YandexTokenURL     string // YANDEX_TOKEN_URL
	YandexInfoURL      string // YANDEX_INFO_URL
}

// Load читает окружение и проверяет обязательные значения.
func Load() (Config, error) {
	cfg := Config{
		Port:        envOr("PORT", "8080"),
		DatabaseURL: envOr("DATABASE_URL", defaultDatabaseURL),
		Env:         envOr("ENV", "dev"),

		JWTSecret:       os.Getenv("JWT_SECRET"),
		AccessTokenTTL:  envDuration("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL: envDuration("REFRESH_TOKEN_TTL", 30*24*time.Hour),

		YandexClientID:     os.Getenv("YANDEX_CLIENT_ID"),
		YandexClientSecret: os.Getenv("YANDEX_CLIENT_SECRET"),
		YandexRedirectURL:  envOr("YANDEX_REDIRECT_URL", "http://localhost:8080/auth/yandex/callback"),
		YandexAuthURL:      envOr("YANDEX_AUTH_URL", "https://oauth.yandex.ru/authorize"),
		YandexTokenURL:     envOr("YANDEX_TOKEN_URL", "https://oauth.yandex.ru/token"),
		YandexInfoURL:      envOr("YANDEX_INFO_URL", "https://login.yandex.ru/info"),
	}

	if cfg.Port == "" {
		return cfg, errors.New("config: PORT пуст")
	}
	if _, err := strconv.Atoi(cfg.Port); err != nil {
		return cfg, fmt.Errorf("config: PORT не число: %w", err)
	}
	if cfg.DatabaseURL == "" {
		return cfg, errors.New("config: DATABASE_URL пуст")
	}
	if cfg.JWTSecret == "" {
		if cfg.Env == "prod" {
			return cfg, errors.New("config: JWT_SECRET обязателен в prod")
		}
		cfg.JWTSecret = defaultJWTDevSecret
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
