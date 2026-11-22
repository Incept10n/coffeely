// Package server — сборка HTTP-сервера coffeely: health-пробы и все домены.
package server

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Incept10n/coffeely/backend/internal/admin"
	"github.com/Incept10n/coffeely/backend/internal/appversion"
	"github.com/Incept10n/coffeely/backend/internal/auth"
	"github.com/Incept10n/coffeely/backend/internal/config"
	"github.com/Incept10n/coffeely/backend/internal/email"
	"github.com/Incept10n/coffeely/backend/internal/httpx"
	"github.com/Incept10n/coffeely/backend/internal/orders"
	"github.com/Incept10n/coffeely/backend/internal/points"
	"github.com/Incept10n/coffeely/backend/internal/securebox"
	"github.com/Incept10n/coffeely/backend/internal/superadmin"
)

// New собирает mux со всеми маршрутами; каждый запрос пишется в stdout.
func New(pool *pgxpool.Pool, cfg config.Config) http.Handler {
	var mailer auth.CodeSender
	if cfg.MailPassword != "" {
		mailer = email.NewEWS(cfg)
	}
	return NewWithMailer(pool, cfg, mailer)
}

// NewWithMailer — вариант с явным отправителем писем (тесты с фейковой почтой).
func NewWithMailer(pool *pgxpool.Pool, cfg config.Config, mailer auth.CodeSender) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"coffeely-backend"}`))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		pingCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		w.Header().Set("Content-Type", "application/json")
		if err := pool.Ping(pingCtx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"unready","db":false}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ready","db":true}`))
	})

	// Аутентификация (веха M2)
	authStore := auth.NewStore(pool)
	authHandlers := auth.NewHandlers(
		auth.NewService(authStore, cfg, mailer),
		auth.NewYandex(cfg),
		cfg,
	)
	authenticate := authHandlers.Authenticate

	mux.HandleFunc("POST /auth/register", authHandlers.Register)
	mux.HandleFunc("POST /auth/verify-email", authHandlers.VerifyEmail)
	mux.HandleFunc("POST /auth/resend-verification", authHandlers.ResendVerification)
	mux.HandleFunc("POST /auth/login", authHandlers.Login)
	mux.HandleFunc("POST /auth/refresh", authHandlers.Refresh)
	mux.HandleFunc("POST /auth/logout", authHandlers.Logout)
	mux.Handle("GET /auth/me", authenticate(http.HandlerFunc(authHandlers.Me)))
	mux.HandleFunc("GET /auth/yandex/start", authHandlers.YandexStart)
	mux.HandleFunc("GET /auth/yandex/callback", authHandlers.YandexCallback)
	// Вход через Яндекс ID из мобильного приложения (LoginSDK): токен → сессия.
	mux.HandleFunc("POST /auth/yandex/token", authHandlers.YandexToken)
	mux.HandleFunc("GET /auth/providers", authHandlers.Providers)

	// Кабинет владельца на сайте: регистрация и вход (только веб, не приложение).
	mux.HandleFunc("POST /owners/register", authHandlers.OwnersRegister)
	mux.HandleFunc("POST /owners/login", authHandlers.OwnersLogin)

	// Админка владельца: свои точки, меню и рабочие аккаунты бариста.
	// Обратимые копии паролей бариста шифруются ключом из JWT_SECRET.
	adminHandlers := admin.NewHandlers(admin.NewStore(pool), securebox.New(cfg.JWTSecret))
	ownerOnly := adminHandlers.RequireOwner(authenticate)
	mux.Handle("POST /admin/points", ownerOnly(http.HandlerFunc(adminHandlers.CreatePoint)))
	mux.Handle("GET /admin/points", ownerOnly(http.HandlerFunc(adminHandlers.ListPoints)))
	mux.Handle("PATCH /admin/points/{id}", ownerOnly(http.HandlerFunc(adminHandlers.UpdatePoint)))
	mux.Handle("GET /admin/points/{id}/menu", ownerOnly(http.HandlerFunc(adminHandlers.ListMenu)))
	mux.Handle("POST /admin/points/{id}/menu", ownerOnly(http.HandlerFunc(adminHandlers.CreateMenuItem)))
	mux.Handle("PATCH /admin/menu/{id}", ownerOnly(http.HandlerFunc(adminHandlers.UpdateMenuItem)))
	mux.Handle("POST /admin/menu/{id}/addons", ownerOnly(http.HandlerFunc(adminHandlers.CreateAddon)))
	mux.Handle("PATCH /admin/addons/{id}", ownerOnly(http.HandlerFunc(adminHandlers.UpdateAddon)))
	mux.Handle("DELETE /admin/addons/{id}", ownerOnly(http.HandlerFunc(adminHandlers.DeleteAddon)))
	mux.Handle("GET /admin/profile", ownerOnly(http.HandlerFunc(adminHandlers.Profile)))
	mux.Handle("PATCH /admin/profile", ownerOnly(http.HandlerFunc(adminHandlers.UpdateProfile)))
	mux.Handle("POST /admin/baristas", ownerOnly(http.HandlerFunc(adminHandlers.CreateBarista)))
	mux.Handle("GET /admin/baristas", ownerOnly(http.HandlerFunc(adminHandlers.ListBaristas)))
	mux.Handle("PATCH /admin/baristas/{id}", ownerOnly(http.HandlerFunc(adminHandlers.UpdateBarista)))

	// Внутренняя админка создателей бизнеса: /admin на сайте, вход по общему
	// секрету ADMIN_CRM_SECRET (не путать с кабинетом владельца /admin/*).
	superHandlers := superadmin.NewHandlers(superadmin.NewStore(pool), cfg.AdminCRMSecret)
	mux.HandleFunc("POST /internal/admin/login", superHandlers.Login)
	mux.Handle("GET /internal/admin/analytics",
		superHandlers.RequireSecret(http.HandlerFunc(superHandlers.Analytics)))

	// Реестр сборок приложения: публичная проверка обновлений + регистрация
	// новых APK из CI и аварийная блокировка версии (X-Admin-Secret).
	appVersionHandlers := appversion.NewHandlers(appversion.NewStore(pool))
	mux.HandleFunc("GET /app/version", appVersionHandlers.Version)
	mux.Handle("POST /internal/admin/app-versions",
		superHandlers.RequireSecret(http.HandlerFunc(appVersionHandlers.Publish)))
	mux.Handle("PATCH /internal/admin/app-versions/{platform}/{version_code}",
		superHandlers.RequireSecret(http.HandlerFunc(appVersionHandlers.SetMandatory)))

	// Заказы, KDS-очередь, realtime (M3) + клиентский флоу (M4)
	pointsStore := points.NewStore(pool)
	ordersHub := orders.NewHub()
	ordersHandlers := orders.NewHandlers(orders.NewStore(pool, pointsStore.PlanSlot), ordersHub)
	mux.Handle("POST /orders", authenticate(http.HandlerFunc(ordersHandlers.Create)))
	mux.Handle("GET /orders", authenticate(http.HandlerFunc(ordersHandlers.List)))
	mux.Handle("GET /orders/{id}", authenticate(http.HandlerFunc(ordersHandlers.ByID)))
	mux.Handle("GET /barista/queue", authenticate(http.HandlerFunc(ordersHandlers.Queue)))
	mux.Handle("POST /barista/orders/{id}/status", authenticate(http.HandlerFunc(ordersHandlers.SetStatus)))
	mux.Handle("GET /barista/metrics", authenticate(http.HandlerFunc(ordersHandlers.Metrics)))
	mux.Handle("GET /ws/points/{id}", authenticate(http.HandlerFunc(ordersHandlers.PointWS)))
	mux.Handle("GET /ws/orders", authenticate(http.HandlerFunc(ordersHandlers.UserWS)))

	pointsHandlers := points.NewHandlers(pointsStore)
	mux.Handle("GET /points", authenticate(http.HandlerFunc(pointsHandlers.List)))
	mux.Handle("GET /points/{id}/menu", authenticate(http.HandlerFunc(pointsHandlers.Menu)))

	return httpx.LogRequests(mux)
}
