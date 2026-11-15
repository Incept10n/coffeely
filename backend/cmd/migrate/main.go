// Command migrate — запуск миграций coffeely без внешнего CLI.
//
// Использование:
//
//	go run ./cmd/migrate up      # поднять
//	go run ./cmd/migrate down    # откатить одну
//	go run ./cmd/migrate drop    # снести всё (dev)
//
// DATABASE_URL берётся из окружения (дефолт — dev-значение из config).
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/Incept10n/coffeely/backend/internal/config"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: migrate up|down|drop")
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	m, err := migrate.New("file://migrations", cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil || dbErr != nil {
			fmt.Fprintln(os.Stderr, "migrate: close:", srcErr, dbErr)
		}
	}()

	switch os.Args[1] {
	case "up":
		err = m.Up()
	case "down":
		err = m.Steps(-1)
	case "drop":
		err = m.Drop()
	default:
		fmt.Fprintln(os.Stderr, "usage: migrate up|down|drop")
		os.Exit(2)
	}

	if err != nil {
		// «нечего применять» — это успех: команда идемпотентна (initContainer,
		// повторный деплой). Раньше возвращала exit 1.
		if errors.Is(err, migrate.ErrNoChange) {
			fmt.Println("migrate: no change")
			return
		}
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}
