// Package server — сборка HTTP-сервера coffeely: health-пробы и все домены.
package server

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Incept10n/coffeely/backend/internal/admin"
	"github.com/Incept10n/coffeely/backend/internal/auth"
	"github.com/Incept10n/coffeely/backend/internal/config"
	"github.com/Incept10n/coffeely/backend/internal/httpx"
	"github.com/Incept10n/coffeely/backend/internal/orders"
	"github.com/Incept10n/coffeely/backend/internal/points"
)

// New собирает mux со всеми маршрутами; каждый запрос пишется в stdout.
func New(pool *pgxpool.Pool, cfg config.Config) http.Handler {
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
		auth.NewService(authStore, cfg),
		auth.NewYandex(cfg),
		cfg,
	)
	authenticate := authHandlers.Authenticate

	mux.HandleFunc("POST /auth/register", authHandlers.Register)
	mux.HandleFunc("POST /auth/login", authHandlers.Login)
	mux.HandleFunc("POST /auth/refresh", authHandlers.Refresh)
	mux.HandleFunc("POST /auth/logout", authHandlers.Logout)
	mux.Handle("GET /auth/me", authenticate(http.HandlerFunc(authHandlers.Me)))
	mux.HandleFunc("GET /auth/yandex/start", authHandlers.YandexStart)
	mux.HandleFunc("GET /auth/yandex/callback", authHandlers.YandexCallback)

	// Кабинет владельца на сайте: регистрация и вход (только веб, не приложение).
	mux.HandleFunc("POST /owners/register", authHandlers.OwnersRegister)
	mux.HandleFunc("POST /owners/login", authHandlers.OwnersLogin)

	// Админка владельца: свои точки и рабочие аккаунты бариста.
	adminHandlers := admin.NewHandlers(admin.NewStore(pool))
	ownerOnly := adminHandlers.RequireOwner(authenticate)
	mux.Handle("POST /admin/points", ownerOnly(http.HandlerFunc(adminHandlers.CreatePoint)))
	mux.Handle("GET /admin/points", ownerOnly(http.HandlerFunc(adminHandlers.ListPoints)))
	mux.Handle("POST /admin/baristas", ownerOnly(http.HandlerFunc(adminHandlers.CreateBarista)))
	mux.Handle("GET /admin/baristas", ownerOnly(http.HandlerFunc(adminHandlers.ListBaristas)))

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
