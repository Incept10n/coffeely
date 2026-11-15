package server

// E2E-тесты клиентского флоу: точки с загрузкой, меню, заказ без выбора окна,
// «мои заказы», назначение окна движком ёмкости.

import (
	"context"
	"encoding/json"
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

func parseTime(t *testing.T, v any) time.Time {
	t.Helper()
	s, _ := v.(string)
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("время %q: %v", s, err)
	}
	return ts
}

// Тест: точки с загрузкой → меню → заказ без окна → окно назначено движком →
// «мои заказы» → второй заказ получает следующее окно → бариста видит очередь.
func TestClientFlowE2E(t *testing.T) {
	e := newTestEnv(t)
	e.srv = httptest.NewServer(e.handler)
	t.Cleanup(e.srv.Close)

	pointTight, _, menuID := seedClientFlow(t, e)
	clientA := registerClient(t, e)
	clientB := registerClient(t, e)

	// список точек: загрузка нулевая, точка открыта
	status, resp := e.doJSON(t, "GET", "/points", clientA, nil)
	if status != http.StatusOK {
		t.Fatalf("points: %d: %v", status, resp)
	}
	points := resp["points"].([]any)
	if len(points) != 2 {
		t.Fatalf("points: ожидали 2, получили %v", resp["points"])
	}
	load := points[0].(map[string]any)["load"].(map[string]any)
	if load["wait_seconds"].(float64) != 0 || load["is_open"] != true {
		t.Fatalf("load пустой точки: %v", load)
	}

	// меню
	status, resp = e.doJSON(t, "GET", "/points/"+pointTight+"/menu", clientA, nil)
	if status != http.StatusOK || len(resp["menu"].([]any)) != 1 {
		t.Fatalf("menu: %d: %v", status, resp)
	}

	// заказ без окна: клиент не выбирает окошки
	before := time.Now()
	status, resp = e.doJSON(t, "POST", "/orders", clientA, map[string]any{
		"point_id": pointTight,
		"items":    []map[string]any{{"menu_item_id": menuID, "qty": 1}},
	})
	if status != http.StatusCreated {
		t.Fatalf("order A: %d: %v", status, resp)
	}
	orderAID, _ := resp["id"].(string)
	if resp["status"] != "paid" {
		t.Fatalf("статус нового заказа: %v", resp["status"])
	}
	if resp["number"].(float64) < 1 {
		t.Fatalf("номер заказа: %v", resp["number"])
	}
	slotStart := parseTime(t, resp["pickup_slot_start"])
	slotEnd := parseTime(t, resp["pickup_slot_end"])
	deadline := parseTime(t, resp["deadline_at"])

	// окно назначено в будущем и выровнено по сетке 5 минут
	if !slotStart.After(before) {
		t.Fatalf("окно в прошлом: %v (сейчас %v)", slotStart, before)
	}
	moscow, _ := time.LoadLocation("Europe/Moscow")
	if slotStart.In(moscow).Minute()%5 != 0 || slotStart.In(moscow).Second() != 0 {
		t.Fatalf("окно не выровнено по сетке 5 минут: %v", slotStart)
	}
	if slotEnd.Sub(slotStart) != 5*time.Minute {
		t.Fatalf("длина окна: %v", slotEnd.Sub(slotStart))
	}
	// точное время готовности = окно − prep (120 с)
	if want := slotStart.Add(-120 * time.Second); !deadline.Equal(want) {
		t.Fatalf("deadline: ожидали %v, получили %v", want, deadline)
	}

	// «мои заказы»: заказ виден клиенту, с названием точки
	status, resp = e.doJSON(t, "GET", "/orders", clientA, nil)
	if status != http.StatusOK {
		t.Fatalf("мои заказы: %d: %v", status, resp)
	}
	myOrders := resp["orders"].([]any)
	if len(myOrders) != 1 {
		t.Fatalf("мои заказы: ожидали 1, получили %d", len(myOrders))
	}
	if title, _ := myOrders[0].(map[string]any)["point_title"].(string); title != "Узкая точка" {
		t.Fatalf("point_title: %v", title)
	}

	// у другого клиента заказов нет
	_, other := e.doJSON(t, "GET", "/orders", clientB, nil)
	if len(other["orders"].([]any)) != 0 {
		t.Fatalf("чужие заказы видны: %v", other["orders"])
	}

	// загрузка точки теперь видна в списке
	_, resp = e.doJSON(t, "GET", "/points", clientB, nil)
	var tightLoad map[string]any
	for _, p := range resp["points"].([]any) {
		pm := p.(map[string]any)
		if pm["id"] == pointTight {
			tightLoad = pm["load"].(map[string]any)
		}
	}
	if tightLoad["wait_seconds"].(float64) != 120 {
		t.Fatalf("загрузка точки: %v", tightLoad)
	}

	// второй заказ: окно не то же (ёмкость 1 на окно + очередь) — клиент не получает отказ
	status, resp = e.doJSON(t, "POST", "/orders", clientB, map[string]any{
		"point_id": pointTight,
		"items":    []map[string]any{{"menu_item_id": menuID, "qty": 1}},
	})
	if status != http.StatusCreated {
		t.Fatalf("order B: %d: %v", status, resp)
	}
	slotB := parseTime(t, resp["pickup_slot_start"])
	if !slotB.After(slotStart) {
		t.Fatalf("второй заказ в то же окно: %v vs %v", slotB, slotStart)
	}

	// статус виден баристе точки
	_, _, _, baristaAccess, _ := seedKDSForPoint(t, e, pointTight)
	status, resp = e.doJSON(t, "GET", "/barista/queue", baristaAccess, nil)
	if status != http.StatusOK {
		t.Fatalf("queue: %d", status)
	}
	if got := len(resp["orders"].([]any)); got != 2 {
		t.Fatalf("queue: ожидали 2 заказа, получили %d", got)
	}

	// смена статуса баристой видна клиенту в его заказе
	status, resp = e.doJSON(t, "POST", "/barista/orders/"+orderAID+"/status", baristaAccess, map[string]string{"status": "in_production"})
	if status != http.StatusOK {
		t.Fatalf("in_production: %d: %v", status, resp)
	}
	_, resp = e.doJSON(t, "GET", "/orders/"+orderAID, clientA, nil)
	if resp["status"] != "in_production" {
		t.Fatalf("клиент не видит статус: %v", resp["status"])
	}
}

// Ёмкость окна: при prep=0 заказы пакуются в окно до лимита, затем едут дальше.
func TestWindowCapacityE2E(t *testing.T) {
	e := newTestEnv(t)
	e.srv = httptest.NewServer(e.handler)
	t.Cleanup(e.srv.Close)

	_, pointDefault, _ := seedClientFlow(t, e)
	ctx := context.Background()
	var menuID string
	if err := e.pool.QueryRow(ctx,
		`INSERT INTO menu_items (point_id, title, price_minor, prep_seconds) VALUES ($1, 'Американо', 18000, 0) RETURNING id`,
		pointDefault,
	).Scan(&menuID); err != nil {
		t.Fatalf("seed menu default: %v", err)
	}

	body := map[string]any{
		"point_id": pointDefault,
		"items":    []map[string]any{{"menu_item_id": menuID, "qty": 1}},
	}
	var slots []time.Time
	for i := 0; i < 4; i++ {
		access := registerClient(t, e)
		status, resp := e.doJSON(t, "POST", "/orders", access, body)
		if status != http.StatusCreated {
			t.Fatalf("заказ %d: %d: %v", i, status, resp)
		}
		slots = append(slots, parseTime(t, resp["pickup_slot_start"]))
	}
	// первые три — одно окно (лимит 3), четвёртый — следующее
	if !slots[0].Equal(slots[2]) {
		t.Fatalf("первые три заказа должны попасть в одно окно: %v", slots)
	}
	if !slots[3].After(slots[2]) {
		t.Fatalf("четвёртый заказ должен уехать в следующее окно: %v", slots)
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
	return "", "", "", access, ""
}
