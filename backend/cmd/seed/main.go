// Command seed — наполнение БД точкой и меню (идемпотентно по названию точки).
// Пользователей не создаёт: клиенты регистрируются сами (POST /auth/register),
// бариста назначается флагом в БД (см. ниже). Роли owner больше нет.
//
// Пример:
//
//	go run ./cmd/seed -point "coffeely на Ленина" -address "ул. Ленина 1" \
//	  -menu "Капучино:25000:180,Латте:27000:150,Американо:18000:90"
//
// Назначить баристу вручную (клиент регистрируется в приложении, потом):
//
//	UPDATE users SET role = 'barista' WHERE email = '...';
//	INSERT INTO barista_points (user_id, point_id)
//	  SELECT u.id, p.id FROM users u, points p
//	  WHERE u.email = '...' AND p.title = '...';
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Incept10n/coffeely/backend/internal/config"
)

func main() {
	pointTitle := flag.String("point", "", "название точки (обязательно)")
	address := flag.String("address", "", "адрес точки")
	tz := flag.String("tz", "Europe/Moscow", "таймзона точки")
	menu := flag.String("menu", "", "меню: \"Название:цена_копейках:prep_секунд,...\"")
	capacity := flag.String("capacity", `{"window_minutes":5,"max_orders_per_window":3,"work_start":"08:00","work_end":"20:00"}`, "capacity_config точки (jsonb)")
	flag.Parse()

	if *pointTitle == "" {
		fmt.Fprintln(os.Stderr, "нужен -point")
		flag.Usage()
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("db ping: %v", err)
	}

	// Точка: по названию, иначе создаём.
	var pointID string
	err = pool.QueryRow(ctx, `SELECT id FROM points WHERE title = $1`, *pointTitle).Scan(&pointID)
	if err != nil {
		if err := pool.QueryRow(ctx,
			`INSERT INTO points (title, address, timezone, capacity_config) VALUES ($1, $2, $3, $4) RETURNING id`,
			*pointTitle, *address, *tz, *capacity,
		).Scan(&pointID); err != nil {
			log.Fatalf("создание точки: %v", err)
		}
		fmt.Printf("точка создана: %s (%s)\n", *pointTitle, pointID)
	} else {
		fmt.Printf("точка уже есть: %s (%s)\n", *pointTitle, pointID)
	}

	// Меню: только отсутствующие позиции (идемпотентно по названию).
	if *menu != "" {
		for _, item := range strings.Split(*menu, ",") {
			parts := strings.Split(strings.TrimSpace(item), ":")
			if len(parts) != 3 {
				log.Fatalf("позиция меню %q: ожидался формат Название:цена_копейках:prep_секунд", item)
			}
			price, err1 := strconv.Atoi(parts[1])
			prep, err2 := strconv.Atoi(parts[2])
			if err1 != nil || err2 != nil {
				log.Fatalf("позиция меню %q: цена и prep — числа", item)
			}
			if _, err := pool.Exec(ctx, `
				INSERT INTO menu_items (point_id, title, price_minor, prep_seconds)
				SELECT $1, $2, $3, $4
				WHERE NOT EXISTS (SELECT 1 FROM menu_items WHERE point_id = $1 AND title = $2)`,
				pointID, parts[0], price, prep,
			); err != nil {
				log.Fatalf("позиция меню %q: %v", parts[0], err)
			}
		}
		fmt.Println("меню синхронизировано")
	}

	fmt.Println("готово: клиенты видят точку в приложении; бариста назначается флагом в БД")
}
