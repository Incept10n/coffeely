package server

// E2E-тесты вехи M4: точки, меню, окна с ёмкостью, заказ клиента.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Incept10n/coffeely/backend/internal/auth"
)

// seedClientFlow: точка с ёмкостью 1 заказ/окно и точка с дефолтной ёмкостью (3).
func seedClientFlow(t *testing.T, e *testEnv) (pointTight, pointDefault, menuID string) {
	t.Helper()
	ctx := context.Background()

	cfgTight, _ := json.Marshal(map[string]any{
		"window_minutes": 5, "max_orders_per_window": 1,
		"work_start": "00:00", "work_end": "24:00",
	})
	if err := e.pool.QueryRow(ctx,
		`INSERT INTO points (title, capacity_config) VALUES ('Узкая точка', $1) RETURNING id`, cfgTight,
	).Scan(&pointTight); err != nil {
		t.Fatalf("seed tight point: %v", err)
	}
	if err := e.pool.QueryRow(ctx,
		`INSERT INTO points (title, capacity_config) VALUES ('Дефолтная точка', '{}') RETURNING id`,
	).Scan(&pointDefault); err != nil {
		t.Fatalf("seed default point: %v", err)
	}
	if err := e.pool.QueryRow(ctx,
		`INSERT INTO menu_items (point_id, title, price_minor, prep_seconds) VALUES ($1, 'Эспрессо', 15000, 120) RETURNING id`,
		pointTight,
	).Scan(&menuID); err != nil {
		t.Fatalf("seed menu: %v", err)
	}
	return pointTight, pointDefault, menuID
}

func registerClient(t *testing.T, e *testEnv) string {
	t.Helper()
	status, resp := e.doJSON(t, "POST", "/auth/register", "", map[string]string{"email": uniqueEmail(), "password": "supersecret1"})
	if status != http.StatusCreated {
		t.Fatalf("register: %d", status)
	}
	access, _ := tokensFrom(t, resp)
	return access
}

// freeWindow находит будущее окно со свободным местом (день сегодня, иначе завтра).
func freeWindow(t *testing.T, e *testEnv, access, pointID string) map[string]any {
	t.Helper()
	for _, dayOffset := range []time.Duration{0, 24 * time.Hour} {
		day := timeNowAdd(dayOffset)
		status, resp := e.doJSON(t, "GET", "/points/"+pointID+"/windows?day="+day.Format("2006-01-02"), access, nil)
		if status != http.StatusOK {
			t.Fatalf("windows: %d: %v", status, resp)
		}
		windows, _ := resp["windows"].([]any)
		for _, w := range windows {
			win := w.(map[string]any)
			if win["passed"].(bool) {
				continue
			}
			if win["free"].(float64) >= 1 {
				return win
			}
		}
	}
	t.Fatal("не найдено будущее окно со свободной ёмкостью")
	return nil
}

func timeNowAdd(d time.Duration) time.Time { return time.Now().Add(d) }

// Тест: список точек → меню → окно → заказ; второй клиент на то же окно получает отказ.
func TestClientFlowE2E(t *testing.T) {
	e := newTestEnv(t)
	e.srv = httptest.NewServer(e.handler)
	t.Cleanup(e.srv.Close)

	pointTight, _, menuID := seedClientFlow(t, e)
	clientA := registerClient(t, e)
	clientB := registerClient(t, e)

	// список точек
	status, resp := e.doJSON(t, "GET", "/points", clientA, nil)
	if status != http.StatusOK {
		t.Fatalf("points: %d: %v", status, resp)
	}
	if len(resp["points"].([]any)) != 2 {
		t.Fatalf("points: ожидали 2, получили %v", resp["points"])
	}

	// меню
	status, resp = e.doJSON(t, "GET", "/points/"+pointTight+"/menu", clientA, nil)
	if status != http.StatusOK || len(resp["menu"].([]any)) != 1 {
		t.Fatalf("menu: %d: %v", status, resp)
	}

	// окна: первое будущее со свободным местом
	win := freeWindow(t, e, clientA, pointTight)
	start, end := win["start"].(string), win["end"].(string)

	orderBody := map[string]any{
		"point_id":          pointTight,
		"pickup_slot_start": start,
		"pickup_slot_end":   end,
		"items":             []map[string]any{{"menu_item_id": menuID, "qty": 1}},
	}

	// клиент A берёт окно
	status, resp = e.doJSON(t, "POST", "/orders", clientA, orderBody)
	if status != http.StatusCreated {
		t.Fatalf("order A: %d: %v", status, resp)
	}
	_ = resp["id"].(string) // orderA не нужен далее — проверяем число и дедлайн
	orderANumber := int64(resp["number"].(float64))
	if orderANumber < 1 {
		t.Fatalf("order A number: %v", resp["number"])
	}
	// точное время готовности: дедлайн = окно − prep (120s)
	deadline, err := time.Parse(time.RFC3339, resp["deadline_at"].(string))
	if err != nil {
		t.Fatalf("deadline: %v", err)
	}
	slotStart, _ := time.Parse(time.RFC3339, start)
	if want := slotStart.Add(-120 * time.Second); !deadline.Equal(want) {
		t.Fatalf("deadline: ожидали %v, получили %v", want, deadline)
	}

	// клиент B на то же окно → отказ (ёмкость исчерпана)
	status, resp = e.doJSON(t, "POST", "/orders", clientB, orderBody)
	if status != http.StatusConflict {
		t.Fatalf("order B same window: ждём 409, получили %d: %v", status, resp)
	}
	if code, _ := resp["code"].(string); code != "window_full" {
		t.Fatalf("order B: код ошибки: %v", resp["code"])
	}

	// клиент B берёт другое окно — проходит
	win2 := freeWindow(t, e, clientB, pointTight)
	if win2["start"] == win["start"] {
		t.Fatal("окна совпали — тест некорректен")
	}
	orderBody2 := map[string]any{
		"point_id":          pointTight,
		"pickup_slot_start": win2["start"].(string),
		"pickup_slot_end":   win2["end"].(string),
		"items":             []map[string]any{{"menu_item_id": menuID, "qty": 1}},
	}
	status, _ = e.doJSON(t, "POST", "/orders", clientB, orderBody2)
	if status != http.StatusCreated {
		t.Fatalf("order B other window: %d", status)
	}

	// статус виден баристе точки (seed баристы как в M3)
	_, _, _, baristaAccess, _ := seedKDSForPoint(t, e, pointTight)
	status, resp = e.doJSON(t, "GET", "/barista/queue", baristaAccess, nil)
	if status != http.StatusOK {
		t.Fatalf("queue: %d", status)
	}
	if got := len(resp["orders"].([]any)); got != 2 {
		t.Fatalf("queue: ожидали 2 заказа, получили %d", got)
	}
}

// Дефолтная ёмкость (пустой capacity_config): 3 заказа на окно, 4-й — отказ.
func TestDefaultCapacityE2E(t *testing.T) {
	e := newTestEnv(t)
	e.srv = httptest.NewServer(e.handler)
	t.Cleanup(e.srv.Close)

	_, pointDefault, _ := seedClientFlow(t, e)
	// отдельное меню для дефолтной точки (в seedClientFlow меню только у узкой)
	ctx := context.Background()
	var menuID string
	if err := e.pool.QueryRow(ctx,
		`INSERT INTO menu_items (point_id, title, price_minor, prep_seconds) VALUES ($1, 'Американо', 18000, 60) RETURNING id`,
		pointDefault,
	).Scan(&menuID); err != nil {
		t.Fatalf("seed menu default: %v", err)
	}

	clients := []string{registerClient(t, e), registerClient(t, e), registerClient(t, e), registerClient(t, e)}
	win := freeWindow(t, e, clients[0], pointDefault)

	orderBody := map[string]any{
		"point_id":          pointDefault,
		"pickup_slot_start": win["start"].(string),
		"pickup_slot_end":   win["end"].(string),
		"items":             []map[string]any{{"menu_item_id": menuID, "qty": 1}},
	}

	for i, access := range clients[:3] {
		status, resp := e.doJSON(t, "POST", "/orders", access, orderBody)
		if status != http.StatusCreated {
			t.Fatalf("заказ %d: %d: %v", i, status, resp)
		}
	}
	status, resp := e.doJSON(t, "POST", "/orders", clients[3], orderBody)
	if status != http.StatusConflict {
		t.Fatalf("заказ 4: ждём 409, получили %d: %v", status, resp)
	}
}

// seedKDSForPoint — как seedKDS, но для готовой точки.
func seedKDSForPoint(t *testing.T, e *testEnv, pointID string) (_, _, _, baristaAccess, _ string) {
	t.Helper()
	var baristaID string
	if err := e.pool.QueryRow(context.Background(),
		`INSERT INTO users (email, role, password_hash) VALUES ($1, 'barista', $2) RETURNING id`,
		uniqueEmail(), "$argon2id$v=19$m=65536,t=3,p=4$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	).Scan(&baristaID); err != nil {
		t.Fatalf("seed barista: %v", err)
	}
	if _, err := e.pool.Exec(context.Background(),
		`INSERT INTO barista_points (user_id, point_id) VALUES ($1, $2)`, baristaID, pointID,
	); err != nil {
		t.Fatalf("seed barista point: %v", err)
	}
	access, err := auth.IssueAccessToken(e.cfg, baristaID, "barista", "barista-m4@test.coffeely")
	if err != nil {
		t.Fatalf("barista token: %v", err)
	}
	return fmt.Sprintf("%p", e), "", "", access, ""
}
