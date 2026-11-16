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

	// AdminCRMSecret — строка доступа во внутреннюю админку создателей (/admin).
	// Общий секрет: вход без пользователей и ролей. Не задан — админка выключена.
	AdminCRMSecret string // ADMIN_CRM_SECRET

	// Отправка писем через Exchange Web Services (EWS) с NTLM.
	// Пароль пуст — подтверждение email выключено (dev): регистрация сразу
	// выдаёт токены, как раньше. В prod пароль обязателен.
	MailEWSURL             string        // MAIL_EWS_URL
	MailUsername           string        // MAIL_USERNAME
	MailPassword           string        // MAIL_PASSWORD
	MailFrom               string        // MAIL_FROM
	MailFromName           string        // MAIL_FROM_NAME
	MailInsecureSkipVerify bool          // MAIL_INSECURE_SKIP_VERIFY (по умолчанию true)
	MailTimeout            time.Duration // MAIL_TIMEOUT

	// Код подтверждения email после регистрации в приложении.
	VerificationCodeTTL      time.Duration // VERIFICATION_CODE_TTL (15m)
	VerificationResendPeriod time.Duration // VERIFICATION_RESEND_PERIOD (60s)
	VerificationMaxAttempts  int           // VERIFICATION_MAX_ATTEMPTS (5)
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

		AdminCRMSecret: os.Getenv("ADMIN_CRM_SECRET"),

		MailEWSURL:             envOr("MAIL_EWS_URL", "https://mail.inceptech.ru/EWS/Exchange.asmx"),
		MailUsername:           envOr("MAIL_USERNAME", "watchly@inceptech.ru"),
		MailPassword:           os.Getenv("MAIL_PASSWORD"),
		MailFrom:               envOr("MAIL_FROM", "watchly@inceptech.ru"),
		MailFromName:           envOr("MAIL_FROM_NAME", "coffeely"),
		MailInsecureSkipVerify: envBool("MAIL_INSECURE_SKIP_VERIFY", true),
		MailTimeout:            envDuration("MAIL_TIMEOUT", 10*time.Second),

		VerificationCodeTTL:      envDuration("VERIFICATION_CODE_TTL", 15*time.Minute),
		VerificationResendPeriod: envDuration("VERIFICATION_RESEND_PERIOD", time.Minute),
		VerificationMaxAttempts:  envInt("VERIFICATION_MAX_ATTEMPTS", 5),
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
	if cfg.Env == "prod" && cfg.MailPassword == "" {
		return cfg, errors.New("config: MAIL_PASSWORD обязателен в prod (подтверждение email)")
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

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
