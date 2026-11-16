package server

// E2E внутренней админки создателей: доступ только по ADMIN_CRM_SECRET.
// Требует Postgres (как остальные e2e), без БД тест скипается.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSuperadminE2E(t *testing.T) {
	e := newTestEnv(t)
	e.srv = httptest.NewServer(e.handler)
	t.Cleanup(e.srv.Close)

	get := func(path, secret string) (int, map[string]any) {
		t.Helper()
		req, err := http.NewRequest("GET", e.srv.URL+path, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		if secret != "" {
			req.Header.Set("X-Admin-Secret", secret)
		}
		resp, err := e.srv.Client().Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}

	postLogin := func(secret string) (int, map[string]any) {
		t.Helper()
		body := `{"secret":` + strconvQuote(secret) + `}`
		req, err := http.NewRequest("POST", e.srv.URL+"/internal/admin/login", strings.NewReader(body))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := e.srv.Client().Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}

	// Без заголовка и с неверным секретом — 401.
	if status, _ := get("/internal/admin/analytics", ""); status != http.StatusUnauthorized {
		t.Fatalf("analytics без секрета: ждём 401, получили %d", status)
	}
	if status, _ := get("/internal/admin/analytics", "nope"); status != http.StatusUnauthorized {
		t.Fatalf("analytics с чужим секретом: ждём 401, получили %d", status)
	}

	// Верный секрет — аналитика со всеми срезами.
	status, body := get("/internal/admin/analytics", "test-admin-secret")
	if status != http.StatusOK {
		t.Fatalf("analytics: статус %d: %v", status, body)
	}
	window, ok := body["window"].(map[string]any)
	if !ok || window["days"] != float64(30) {
		t.Fatalf("analytics: window %v", body["window"])
	}
	daily, ok := body["daily"].([]any)
	if !ok || len(daily) != 30 {
		t.Fatalf("analytics: daily len %d", len(daily))
	}
	if _, ok := body["totals"].(map[string]any); !ok {
		t.Fatalf("analytics: нет totals: %v", body)
	}
	if hours, ok := body["hours"].([]any); !ok || len(hours) != 24 {
		t.Fatalf("analytics: hours %v", body["hours"])
	}

	// Окно настраивается, кривое значение — 400.
	status, body = get("/internal/admin/analytics?days=7", "test-admin-secret")
	if status != http.StatusOK {
		t.Fatalf("analytics days=7: статус %d: %v", status, body)
	}
	if window, _ := body["window"].(map[string]any); window["days"] != float64(7) {
		t.Fatalf("analytics days=7: окно %v", body["window"])
	}
	if daily, _ := body["daily"].([]any); len(daily) != 7 {
		t.Fatalf("analytics days=7: daily len %d", len(daily))
	}
	if status, _ := get("/internal/admin/analytics?days=0", "test-admin-secret"); status != http.StatusBadRequest {
		t.Fatalf("analytics days=0: ждём 400, получили %d", status)
	}

	// Логин по строке из тела.
	if status, _ := postLogin("nope"); status != http.StatusUnauthorized {
		t.Fatalf("login с чужим секретом: ждём 401, получили %d", status)
	}
	if status, body := postLogin("test-admin-secret"); status != http.StatusOK || body["ok"] != true {
		t.Fatalf("login: %d: %v", status, body)
	}
}

func strconvQuote(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}

// Тест с реальными данными: проверяем, что сводка считает деньги, дедлайны,
// отмены и воронку, а не просто отдаёт нули на пустой базе.
func TestSuperadminAnalyticsDataE2E(t *testing.T) {
	e := newTestEnv(t)
	e.srv = httptest.NewServer(e.handler)
	t.Cleanup(e.srv.Close)
	ctx := context.Background()

	var ownerID, clientID, baristaID, pointID, menuID string
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := e.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed: %v\n%s", err, sql)
		}
	}
	mustQuery := func(dst *string, sql string, args ...any) {
		t.Helper()
		if err := e.pool.QueryRow(ctx, sql, args...).Scan(dst); err != nil {
			t.Fatalf("seed: %v\n%s", err, sql)
		}
	}

	mustQuery(&ownerID, `INSERT INTO users (role, email, password_hash)
		VALUES ('owner', 'owner@test.coffeely', 'x') RETURNING id`)
	mustQuery(&clientID, `INSERT INTO users (role, email, password_hash)
		VALUES ('client', 'client@test.coffeely', 'x') RETURNING id`)
	mustQuery(&baristaID, `INSERT INTO users (role, email, password_hash)
		VALUES ('barista', 'barista@test.coffeely', 'x') RETURNING id`)
	mustQuery(&pointID, `INSERT INTO points (title, address, owner_id, capacity_config)
		VALUES ('Точка', 'ул. 1', $1,
		        '{"window_minutes":5,"max_orders_per_window":3,"work_start":"08:00","work_end":"20:00"}')
		RETURNING id`, ownerID)
	mustExec(`INSERT INTO barista_points (user_id, point_id) VALUES ($1, $2)`, baristaID, pointID)
	mustQuery(&menuID, `INSERT INTO menu_items (point_id, title, price_minor, prep_seconds)
		VALUES ($1, 'Капучино', 30000, 180) RETURNING id`, pointID)

	mkOrder := func(status string, total int, deadlineAgo, outAgo string) string {
		var id string
		mustQuery(&id, `INSERT INTO orders
			(point_id, client_id, status, pickup_slot_start, pickup_slot_end, deadline_at, total_minor)
			VALUES ($1, $2, $3::order_status, now() - interval '3 hours', now(),
			        now() - $4::interval, $5)
			RETURNING id`, pointID, clientID, status, deadlineAgo, total)
		if outAgo != "" {
			mustExec(`INSERT INTO order_events (order_id, event, at, actor_id)
				VALUES ($1, 'in_production', now() - interval '2 hours', $2),
				       ($1, 'ready', now() - interval '90 minutes', $2),
				       ($1, 'given_out', now() - $3::interval, $2)`,
				id, baristaID, outAgo)
		}
		return id
	}

	// Вовремя: дедлайн час назад, выдан два часа назад.
	o1 := mkOrder("given_out", 60000, "1 hour", "2 hours")
	// С опозданием на 30 минут.
	o2 := mkOrder("given_out", 30000, "1 hour", "30 minutes")
	// Отменён.
	_ = mkOrder("cancelled", 25000, "1 hour", "")

	mustExec(`INSERT INTO order_items (order_id, menu_item_id, qty, price_minor)
		VALUES ($1, $3, 2, 30000), ($2, $3, 1, 30000)`, o1, o2, menuID)

	req, err := http.NewRequest("GET", e.srv.URL+"/internal/admin/analytics?days=30", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("X-Admin-Secret", "test-admin-secret")
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		Totals struct {
			Owners             int64   `json:"owners"`
			Clients            int64   `json:"clients"`
			Baristas           int64   `json:"baristas"`
			Points             int64   `json:"points"`
			OrdersWindow       int64   `json:"orders_window"`
			RevenueWindowMinor int64   `json:"revenue_window_minor"`
			BookedWindowMinor  int64   `json:"booked_window_minor"`
			AOVMinor           int64   `json:"aov_minor"`
			GivenOutWindow     int64   `json:"given_out_window"`
			CancelledWindow    int64   `json:"cancelled_window"`
			CancelRatePct      float64 `json:"cancel_rate_pct"`
			OnTimeWindow       int64   `json:"on_time_window"`
			MissedWindow       int64   `json:"missed_window"`
			OnTimePct          float64 `json:"on_time_pct"`
			AvgLatenessSeconds int64   `json:"avg_lateness_seconds"`
			RepeatClients      int64   `json:"repeat_clients"`
			ClientsWithOrders  int64   `json:"clients_with_orders"`
		} `json:"totals"`
		Points []struct {
			OrdersWindow int64   `json:"orders_window"`
			OnTime       int64   `json:"on_time"`
			Missed       int64   `json:"missed"`
			OnTimePct    float64 `json:"on_time_pct"`
			Baristas     int64   `json:"baristas"`
		} `json:"points"`
		TopItems []struct {
			Title        string `json:"title"`
			Qty          int64  `json:"qty"`
			RevenueMinor int64  `json:"revenue_minor"`
		} `json:"top_items"`
		Baristas []struct {
			Closed int64 `json:"closed"`
		} `json:"baristas"`
		Activation struct {
			ClientsNoOrders int64 `json:"clients_no_orders"`
		} `json:"activation"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	tt := body.Totals
	if tt.Owners != 1 || tt.Clients != 1 || tt.Baristas != 1 || tt.Points != 1 {
		t.Fatalf("totals users/points: %+v", tt)
	}
	if tt.OrdersWindow != 3 {
		t.Fatalf("orders_window: ждём 3, %d", tt.OrdersWindow)
	}
	if tt.RevenueWindowMinor != 90000 || tt.BookedWindowMinor != 90000 || tt.AOVMinor != 45000 {
		t.Fatalf("деньги: revenue=%d booked=%d aov=%d",
			tt.RevenueWindowMinor, tt.BookedWindowMinor, tt.AOVMinor)
	}
	if tt.GivenOutWindow != 2 || tt.CancelledWindow != 1 || tt.CancelRatePct != 33.3 {
		t.Fatalf("выдачи/отмены: %+v", tt)
	}
	if tt.OnTimeWindow != 1 || tt.MissedWindow != 1 || tt.OnTimePct != 50 {
		t.Fatalf("дедлайны: on_time=%d missed=%d pct=%v",
			tt.OnTimeWindow, tt.MissedWindow, tt.OnTimePct)
	}
	if tt.AvgLatenessSeconds != 1800 {
		t.Fatalf("среднее опоздание: %d", tt.AvgLatenessSeconds)
	}
	if body.Totals.ClientsWithOrders != 1 || body.Totals.RepeatClients != 1 {
		t.Fatalf("повторные: %+v", body.Totals)
	}

	if len(body.Points) != 1 {
		t.Fatalf("points: %d", len(body.Points))
	}
	p := body.Points[0]
	if p.OrdersWindow != 3 || p.OnTime != 1 || p.Missed != 1 || p.OnTimePct != 50 || p.Baristas != 1 {
		t.Fatalf("point row: %+v", p)
	}

	if len(body.TopItems) != 1 || body.TopItems[0].Qty != 3 || body.TopItems[0].RevenueMinor != 90000 {
		t.Fatalf("top items: %+v", body.TopItems)
	}
	if len(body.Baristas) != 1 || body.Baristas[0].Closed != 2 {
		t.Fatalf("baristas: %+v", body.Baristas)
	}
	if body.Activation.ClientsNoOrders != 0 {
		t.Fatalf("активация: %+v", body.Activation)
	}
}
