// Package config — конфигурация backend из переменных окружения.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Dev-дефолт совпадает с deploy/dev/.env.example; это не секрет, а локальная dev-сборка.
const defaultDatabaseURL = "postgres://coffeely:coffeely@localhost:5432/coffeely?sslmode=disable"

// Config — все настройки сервера, читаемые из env.
type Config struct {
	Port        string        // PORT
	DatabaseURL string        // DATABASE_URL
	Env         string        // ENV: dev / prod
	ReadTimeout time.Duration // READ_TIMEOUT_SECONDS
}

// Load читает окружение и проверяет обязательные значения.
func Load() (Config, error) {
	cfg := Config{
		Port:        envOr("PORT", "8080"),
		DatabaseURL: envOr("DATABASE_URL", defaultDatabaseURL),
		Env:         envOr("ENV", "dev"),
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
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
