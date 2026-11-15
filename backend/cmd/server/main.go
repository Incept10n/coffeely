// Command server — точка входа API coffeely.
// Минимальный HTTP-сервер с health-пробами и подключением к PostgreSQL.
// HTTP-роутер (chi) и домены подключаются следующими вехами.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Incept10n/coffeely/backend/internal/config"
	"github.com/Incept10n/coffeely/backend/internal/db"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config", "err", err)
		os.Exit(1)
	}

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("db connect", "err", err)
		os.Exit(1)
	}
	logger.Info("db connected")

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

	addr := ":" + cfg.Port
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("server starting", "addr", addr, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("listen failed", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown failed", "err", err)
	}
	pool.Close()
	logger.Info("bye")
}
