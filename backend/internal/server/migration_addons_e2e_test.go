package server

// Регрессия миграции 000009: она переносит добавки с точки на товар, поэтому
// проверять её нужно на непустой таблице addons. CI-прогон e2e на чистой базе
// этот случай не ловит: пустой INSERT проходит, а с данными падал на NOT NULL
// addons.point_id (колонка ещё существует до DROP COLUMN) — прод встал на
// dirty version 9. Тест поднимает отдельную БД до версии 8, сеет точку, товары
// и добавку, затем накатывает 000009 и проверяет результат.

import (
	"context"
	"database/sql"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

const migrationTestDSN = "postgres://coffeely:coffeely@localhost:5432/coffeely_migtest?sslmode=disable"

func TestMigration009KeepsAddonsWithExistingData(t *testing.T) {
	ctx := context.Background()

	admin, err := sql.Open("postgres", adminDSN)
	if err != nil {
		t.Skipf("тестовая БД недоступна: %v", err)
	}
	if err := admin.PingContext(ctx); err != nil {
		admin.Close()
		t.Skipf("тестовая БД недоступна: %v", err)
	}
	if _, err := admin.ExecContext(ctx, "DROP DATABASE IF EXISTS coffeely_migtest WITH (FORCE)"); err != nil {
		admin.Close()
		t.Fatalf("drop тестовой БД: %v", err)
	}
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE coffeely_migtest"); err != nil {
		admin.Close()
		t.Fatalf("создание тестовой БД: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS coffeely_migtest WITH (FORCE)")
		admin.Close()
	})

	m, err := migrate.New("file://../../migrations", migrationTestDSN)
	if err != nil {
		t.Fatalf("migrate init: %v", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil || dbErr != nil {
			t.Logf("migrate close: %v %v", srcErr, dbErr)
		}
	}()

	// Схема до появления добавок на уровне товара.
	if err := m.Migrate(8); err != nil {
		t.Fatalf("migrate до версии 8: %v", err)
	}

	db, err := sql.Open("postgres", migrationTestDSN)
	if err != nil {
		t.Fatalf("sql open: %v", err)
	}
	defer db.Close()

	dtx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	var pointID string
	if err := dtx.QueryRowContext(ctx,
		"INSERT INTO points (title) VALUES ('Точка на Арбате') RETURNING id",
	).Scan(&pointID); err != nil {
		t.Fatalf("точка: %v", err)
	}
	for _, title := range []string{"Капучино", "Чай"} {
		if _, err := dtx.ExecContext(ctx,
			"INSERT INTO menu_items (point_id, title, price_minor, prep_seconds) VALUES ($1, $2, 25000, 180)",
			pointID, title,
		); err != nil {
			t.Fatalf("товар %s: %v", title, err)
		}
	}
	if _, err := dtx.ExecContext(ctx,
		"INSERT INTO addons (point_id, title, price_minor, prep_seconds) VALUES ($1, 'Сироп карамель', 4000, 10)",
		pointID,
	); err != nil {
		t.Fatalf("добавка: %v", err)
	}
	if err := dtx.Commit(); err != nil {
		t.Fatalf("commit seed: %v", err)
	}

	// Собственно миграция: старая добавка должна разойтись по обоим товарам.
	if err := m.Up(); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	var total int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM addons").Scan(&total); err != nil {
		t.Fatalf("count addons: %v", err)
	}
	if total != 2 {
		t.Fatalf("добавок после миграции: %d, хотим 2 (по одной на каждый товар)", total)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT mi.title, a.title
		FROM addons a
		JOIN menu_items mi ON mi.id = a.menu_item_id
		ORDER BY mi.title`)
	if err != nil {
		t.Fatalf("select addons: %v", err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var item, addon string
		if err := rows.Scan(&item, &addon); err != nil {
			t.Fatalf("scan addons: %v", err)
		}
		got[item] = addon
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	for _, item := range []string{"Капучино", "Чай"} {
		if got[item] != "Сироп карамель" {
			t.Fatalf("у товара %s добавка %q, хотим «Сироп карамель»", item, got[item])
		}
	}

	// Колонка точки должна исчезнуть, а бренд владельца — появиться.
	var pointColumn int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'addons' AND column_name = 'point_id'`).Scan(&pointColumn); err != nil {
		t.Fatalf("схема addons: %v", err)
	}
	if pointColumn != 0 {
		t.Fatal("addons.point_id должна быть удалена")
	}
	var brandColumn int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'users' AND column_name = 'brand_name'`).Scan(&brandColumn); err != nil {
		t.Fatalf("схема users: %v", err)
	}
	if brandColumn != 1 {
		t.Fatal("users.brand_name должна существовать")
	}

	// Миграция не должна оставлять грязную версию.
	var version int
	var dirty bool
	if err := db.QueryRowContext(ctx, "SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty); err != nil {
		t.Fatalf("schema_migrations: %v", err)
	}
	// Проверяем, что дошли как минимум до 000009 и не оставили грязную версию:
	// точное число меняется с каждой новой миграцией.
	if version < 9 || dirty {
		t.Fatalf("schema_migrations: version=%d dirty=%v, хотим >=9 и dirty=false", version, dirty)
	}
}
