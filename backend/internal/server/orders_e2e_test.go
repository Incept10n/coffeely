package server

// E2E-тесты вехи M3: KDS-очередь, статусы, realtime (WebSocket), метрики дедлайнов.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Incept10n/coffeely/backend/internal/auth"
)

// seedKDS готовит точку, меню, баристу и клиента; возвращает id и токены.
func seedKDS(t *testing.T, e *testEnv) (pointID, menuA, menuB, baristaAccess, clientAccess string) {
	t.Helper()
	ctx := context.Background()

	// Круглосуточная точка: иначе тесты зависят от времени прогона
	// (вне рабочих часов движок ёмкости переносит окно на следующее открытие).
	if err := e.pool.QueryRow(ctx,
		`INSERT INTO points (title, address, capacity_config)
		 VALUES ('Точка KDS', 'ул. КДС 1', '{"work_start":"00:00","work_end":"24:00"}')
		 RETURNING id`,
	).Scan(&pointID); err != nil {
		t.Fatalf("seed point: %v", err)
	}
	if err := e.pool.QueryRow(ctx,
		`INSERT INTO menu_items (point_id, title, price_minor, prep_seconds) VALUES ($1, 'Капучино', 25000, 180) RETURNING id`,
		pointID,
	).Scan(&menuA); err != nil {
		t.Fatalf("seed menu a: %v", err)
	}
	if err := e.pool.QueryRow(ctx,
		`INSERT INTO menu_items (point_id, title, price_minor, prep_seconds) VALUES ($1, 'Латте', 27000, 120) RETURNING id`,
		pointID,
	).Scan(&menuB); err != nil {
		t.Fatalf("seed menu b: %v", err)
	}

	// бариста напрямую в БД (привязка через invite-code покрыта в TestInviteCodesE2E)
	var baristaID string
	if err := e.pool.QueryRow(ctx,
		`INSERT INTO users (email, role, password_hash) VALUES ($1, 'barista', $2) RETURNING id`,
		uniqueEmail(), "$argon2id$v=19$m=65536,t=3,p=4$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	).Scan(&baristaID); err != nil {
		t.Fatalf("seed barista: %v", err)
	}
	if _, err := e.pool.Exec(ctx,
		`INSERT INTO barista_points (user_id, point_id) VALUES ($1, $2)`, baristaID, pointID,
	); err != nil {
		t.Fatalf("seed barista point: %v", err)
	}
	baristaAccess, err := auth.IssueAccessToken(e.cfg, baristaID, "barista", "barista-kds@test.coffeely")
	if err != nil {
		t.Fatalf("barista token: %v", err)
	}

	// клиент через API
	status, resp := e.doJSON(t, "POST", "/auth/register", "", map[string]string{"email": uniqueEmail(), "password": "supersecret1"})
	if status != http.StatusCreated {
		t.Fatalf("register client: %d", status)
	}
	clientAccess, _ = tokensFrom(t, resp)
	return pointID, menuA, menuB, baristaAccess, clientAccess
}

func makeOrder(t *testing.T, e *testEnv, clientAccess, pointID, menuA, menuB string) map[string]any {
	t.Helper()
	items := []map[string]any{{"menu_item_id": menuA, "qty": 1}}
	if menuB != "" {
		items = append(items, map[string]any{"menu_item_id": menuB, "qty": 2})
	}
	status, resp := e.doJSON(t, "POST", "/orders", clientAccess, map[string]any{
		"point_id": pointID,
		"items":    items,
	})
	if status != http.StatusCreated {
		t.Fatalf("create order: статус %d: %v", status, resp)
	}
	return resp
}

// Тест: создание заказа, движок дедлайна, KDS-очередь, права.
func TestOrderAndQueueE2E(t *testing.T) {
	e := newTestEnv(t)
	e.srv = httptest.NewServer(e.handler)
	t.Cleanup(e.srv.Close)

	pointID, menuA, menuB, baristaAccess, clientAccess := seedKDS(t, e)

	// движок дедлайна: сумма prep = 180 + 120*2 = 420 с → окно − 7 минут.
	order := makeOrder(t, e, clientAccess, pointID, menuA, menuB)
	deadline, err := time.Parse(time.RFC3339, order["deadline_at"].(string))
	if err != nil {
		t.Fatalf("deadline parse: %v", err)
	}
	slotStart, err := time.Parse(time.RFC3339, order["pickup_slot_start"].(string))
	if err != nil {
		t.Fatalf("slot parse: %v", err)
	}
	if want := slotStart.Add(-7 * time.Minute); !deadline.Equal(want) {
		t.Fatalf("deadline: ожидали %v, получили %v", want, deadline)
	}
	// окно назначено с запасом на производство: дедлайн не в прошлом
	if deadline.Before(time.Now()) {
		t.Fatalf("дедлайн в прошлом: %v", deadline)
	}
	if order["status"] != "paid" {
		t.Fatalf("статус нового заказа: %v", order["status"])
	}
	if order["total_minor"].(float64) != 25000+27000*2 {
		t.Fatalf("total: %v", order["total_minor"])
	}

	// очередь недоступна клиенту
	if status, _ := e.doJSON(t, "GET", "/barista/queue", clientAccess, nil); status != http.StatusForbidden {
		t.Fatalf("queue клиентом: ждём 403, получили %d", status)
	}

	// очередь баристы: заказ виден, срочность вычислена
	status, resp := e.doJSON(t, "GET", "/barista/queue", baristaAccess, nil)
	if status != http.StatusOK {
		t.Fatalf("queue: статус %d: %v", status, resp)
	}
	list := resp["orders"].([]any)
	if len(list) != 1 {
		t.Fatalf("queue: ожидали 1 заказ, получили %d", len(list))
	}
	entry := list[0].(map[string]any)
	if entry["urgency"] == "" || entry["until_deadline_seconds"] == nil {
		t.Fatalf("queue: нет срочности: %v", entry)
	}

	// недопустимый переход paid → ready
	orderID := order["id"].(string)
	status, resp = e.doJSON(t, "POST", "/barista/orders/"+orderID+"/status", baristaAccess, map[string]string{"status": "ready"})
	if status != http.StatusConflict {
		t.Fatalf("paid→ready: ждём 409, получили %d: %v", status, resp)
	}

	// корректные переходы: paid → in_production → ready → given_out
	for _, next := range []string{"in_production", "ready"} {
		status, resp = e.doJSON(t, "POST", "/barista/orders/"+orderID+"/status", baristaAccess, map[string]string{"status": next})
		if status != http.StatusOK {
			t.Fatalf("переход %s: статус %d: %v", next, status, resp)
		}
	}
	// гость видит статус вторым API-клиентом (поллинг)
	status, resp = e.doJSON(t, "GET", "/orders/"+orderID, clientAccess, nil)
	if status != http.StatusOK || resp["status"] != "ready" {
		t.Fatalf("гость не видит статус: %d %v", status, resp)
	}
	// чужой клиент не видит чужой заказ
	_, other := e.doJSON(t, "POST", "/auth/register", "", map[string]string{"email": uniqueEmail(), "password": "supersecret1"})
	otherAccess, _ := tokensFrom(t, other)
	if status, _ := e.doJSON(t, "GET", "/orders/"+orderID, otherAccess, nil); status != http.StatusForbidden {
		t.Fatalf("чужой заказ: ждём 403, получили %d", status)
	}

	status, _ = e.doJSON(t, "POST", "/barista/orders/"+orderID+"/status", baristaAccess, map[string]string{"status": "given_out"})
	if status != http.StatusOK {
		t.Fatalf("given_out: %d", status)
	}

	// выдан до дедлайна → метрика on_time, без промаха
	status, resp = e.doJSON(t, "GET", "/barista/metrics", baristaAccess, nil)
	if status != http.StatusOK {
		t.Fatalf("metrics: %d: %v", status, resp)
	}
	if resp["on_time"].(float64) != 1 || resp["missed"].(float64) != 0 {
		t.Fatalf("metrics: ожидали 1/0, получили %v", resp)
	}
}

// Тест: realtime — новый заказ и смена статуса приходят баристе по WebSocket.
func TestRealtimeE2E(t *testing.T) {
	e := newTestEnv(t)
	e.srv = httptest.NewServer(e.handler)
	t.Cleanup(e.srv.Close)

	pointID, menuA, _, baristaAccess, clientAccess := seedKDS(t, e)

	// бариста слушает канал точки
	wsURL := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/ws/points/" + pointID
	wsCtx, wsCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer wsCancel()
	conn, _, err := websocket.Dial(wsCtx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + baristaAccess}},
	})
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// новый заказ → событие order_created
	go func() {
		_ = makeOrder(t, e, clientAccess, pointID, menuA, "")
	}()
	msg := readWSMessage(t, conn, wsCtx)
	if msg["type"] != "order_created" {
		t.Fatalf("ws: первое событие не order_created: %v", msg)
	}
	order := msg["order"].(map[string]any)
	orderID := order["id"].(string)

	// смена статуса → order_status
	status, _ := e.doJSON(t, "POST", "/barista/orders/"+orderID+"/status", baristaAccess, map[string]string{"status": "in_production"})
	if status != http.StatusOK {
		t.Fatalf("in_production: %d", status)
	}
	msg = readWSMessage(t, conn, wsCtx)
	if msg["type"] != "order_status" {
		t.Fatalf("ws: второе событие не order_status: %v", msg)
	}
	if got := msg["order"].(map[string]any)["status"]; got != "in_production" {
		t.Fatalf("ws: статус в событии: %v", got)
	}

	// клиент слушает свой канал: статус его заказа приходит без поллинга
	clientWS := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/ws/orders"
	clientCtx, clientCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer clientCancel()
	clientConn, _, err := websocket.Dial(clientCtx, clientWS, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + clientAccess}},
	})
	if err != nil {
		t.Fatalf("ws клиента: %v", err)
	}
	defer clientConn.Close(websocket.StatusNormalClosure, "")

	status, _ = e.doJSON(t, "POST", "/barista/orders/"+orderID+"/status", baristaAccess, map[string]string{"status": "ready"})
	if status != http.StatusOK {
		t.Fatalf("ready: %d", status)
	}
	msg = readWSMessage(t, clientConn, clientCtx)
	if msg["type"] != "order_status" {
		t.Fatalf("ws клиента: событие не order_status: %v", msg)
	}
	if got := msg["order"].(map[string]any)["status"]; got != "ready" {
		t.Fatalf("ws клиента: статус в событии: %v", got)
	}

	// чужая точка → 403
	if status, _ := e.doJSON(t, "GET", "/ws/points/00000000-0000-0000-0000-000000000000", baristaAccess, nil); status != http.StatusForbidden {
		t.Fatalf("ws чужая точка: ждём 403, получили %d", status)
	}
}

// Тест: KDS-борда — категории по статусам: активные заказы со срочностью,
// выданные и отменённые — в архиве дня без срочности.
func TestKDSBoardE2E(t *testing.T) {
	e := newTestEnv(t)
	e.srv = httptest.NewServer(e.handler)
	t.Cleanup(e.srv.Close)

	pointID, menuA, _, baristaAccess, clientAccess := seedKDS(t, e)

	// новый заказ остаётся в категории «нет ответа»
	newOrder := makeOrder(t, e, clientAccess, pointID, menuA, "")

	// заказ, который бариста отменит (первый ответ — отмена)
	_, cancelResp := e.doJSON(t, "POST", "/orders", clientAccess, map[string]any{
		"point_id": pointID,
		"items":    []map[string]any{{"menu_item_id": menuA, "qty": 1}},
	})
	cancelledID := cancelResp["id"].(string)
	if status, resp := e.doJSON(t, "POST", "/barista/orders/"+cancelledID+"/status", baristaAccess, map[string]string{"status": "cancelled"}); status != http.StatusOK {
		t.Fatalf("paid→cancelled: %d: %v", status, resp)
	}

	// заказ, который доведён до выдачи
	done := makeOrder(t, e, clientAccess, pointID, menuA, "")
	doneID := done["id"].(string)
	for _, next := range []string{"in_production", "ready", "given_out"} {
		if status, resp := e.doJSON(t, "POST", "/barista/orders/"+doneID+"/status", baristaAccess, map[string]string{"status": next}); status != http.StatusOK {
			t.Fatalf("переход %s: %d: %v", next, status, resp)
		}
	}

	status, resp := e.doJSON(t, "GET", "/barista/queue", baristaAccess, nil)
	if status != http.StatusOK {
		t.Fatalf("queue: %d: %v", status, resp)
	}
	list := resp["orders"].([]any)
	if len(list) != 3 {
		t.Fatalf("queue: ожидали 3 заказа (новый + выданный + отменённый), получили %d: %v", len(list), resp)
	}

	byStatus := map[string]map[string]any{}
	for _, it := range list {
		entry := it.(map[string]any)
		byStatus[entry["status"].(string)] = entry
	}
	for _, want := range []string{"paid", "given_out", "cancelled"} {
		if byStatus[want] == nil {
			t.Fatalf("queue: нет заказа в статусе %s: %v", want, resp)
		}
	}

	// новый заказ — со срочностью (нужен баристе до первого ответа)
	if byStatus["paid"]["urgency"] == "" || byStatus["paid"]["until_deadline_seconds"] == nil {
		t.Fatalf("queue: у нового заказа нет срочности: %v", byStatus["paid"])
	}
	// архив дня — без срочности (баристе больше нечего решать)
	if byStatus["given_out"]["urgency"] != "" || byStatus["cancelled"]["urgency"] != "" {
		t.Fatalf("queue: у архива дня не пустая срочность: %v", resp)
	}
	if got, ok := byStatus["given_out"]["id"].(string); !ok || got != doneID {
		t.Fatalf("queue: выданный заказ не тот: %v", byStatus["given_out"])
	}
	// архив показывает и состав заказа (бариста видит, что выдал)
	if items, ok := byStatus["given_out"]["items"].([]any); !ok || len(items) == 0 {
		t.Fatalf("queue: у выданного заказа нет позиций: %v", byStatus["given_out"])
	}
	if got, ok := byStatus["cancelled"]["id"].(string); !ok || got != cancelledID {
		t.Fatalf("queue: отменённый заказ не тот: %v", byStatus["cancelled"])
	}
	if got, ok := byStatus["paid"]["id"].(string); !ok || got != newOrder["id"].(string) {
		t.Fatalf("queue: новый заказ не тот: %v", byStatus["paid"])
	}
}

// Тест: статусы можно откатывать назад. Ошибочно выданный заказ возвращается
// к выдаче, отменённый — в работу; события метрик не дублируются.
func TestOrderStatusRollbackE2E(t *testing.T) {
	e := newTestEnv(t)
	e.srv = httptest.NewServer(e.handler)
	t.Cleanup(e.srv.Close)

	pointID, menuA, _, baristaAccess, clientAccess := seedKDS(t, e)
	order := makeOrder(t, e, clientAccess, pointID, menuA, "")
	orderID := order["id"].(string)

	step := func(next string) {
		t.Helper()
		status, resp := e.doJSON(t, "POST", "/barista/orders/"+orderID+"/status", baristaAccess, map[string]string{"status": next})
		if status != http.StatusOK {
			t.Fatalf("переход %s: %d: %v", next, status, resp)
		}
		if got := resp["status"]; got != next {
			t.Fatalf("переход %s: статус в ответе %v", next, got)
		}
	}

	// вперёд и назад по всей цепочке, в конце — откат выдачи к «готов»
	for _, next := range []string{"in_production", "paid", "in_production", "ready", "in_production", "ready", "given_out", "ready"} {
		step(next)
	}

	// после отката выдачи событий given_out не осталось — метрика пустая
	status, resp := e.doJSON(t, "GET", "/barista/metrics", baristaAccess, nil)
	if status != http.StatusOK {
		t.Fatalf("metrics: %d: %v", status, resp)
	}
	if resp["on_time"].(float64) != 0 || resp["missed"].(float64) != 0 {
		t.Fatalf("metrics после отката: ожидали 0/0, получили %v", resp)
	}

	// и снова выдали — метрика считается один раз
	step("given_out")
	status, resp = e.doJSON(t, "GET", "/barista/metrics", baristaAccess, nil)
	if status != http.StatusOK {
		t.Fatalf("metrics: %d: %v", status, resp)
	}
	if resp["on_time"].(float64) != 1 || resp["missed"].(float64) != 0 {
		t.Fatalf("metrics после повторной выдачи: ожидали 1/0, получили %v", resp)
	}

	// отмена и возврат в работу
	other := makeOrder(t, e, clientAccess, pointID, menuA, "")
	otherID := other["id"].(string)
	if status, resp := e.doJSON(t, "POST", "/barista/orders/"+otherID+"/status", baristaAccess, map[string]string{"status": "cancelled"}); status != http.StatusOK {
		t.Fatalf("paid→cancelled: %d: %v", status, resp)
	}
	if status, resp := e.doJSON(t, "POST", "/barista/orders/"+otherID+"/status", baristaAccess, map[string]string{"status": "paid"}); status != http.StatusOK {
		t.Fatalf("cancelled→paid: %d: %v", status, resp)
	}
	status, resp = e.doJSON(t, "GET", "/barista/queue", baristaAccess, nil)
	if status != http.StatusOK {
		t.Fatalf("queue: %d: %v", status, resp)
	}
	found := false
	for _, it := range resp["orders"].([]any) {
		entry := it.(map[string]any)
		if entry["id"] == otherID && entry["status"] == "paid" {
			found = true
		}
	}
	if !found {
		t.Fatalf("queue: возвращённый заказ не в работе: %v", resp)
	}
}

// Тест: промах дедлайна фиксируется и попадает в метрики.
func TestDeadlineMissE2E(t *testing.T) {
	e := newTestEnv(t)
	e.srv = httptest.NewServer(e.handler)
	t.Cleanup(e.srv.Close)

	pointID, menuA, _, baristaAccess, clientAccess := seedKDS(t, e)
	order := makeOrder(t, e, clientAccess, pointID, menuA, "")
	orderID := order["id"].(string)
	// Дедлайн в прошлом: клиент не может заказать в прошлое, поэтому двигаем
	// дедлайн напрямую в БД — проверяем фиксацию промаха к моменту given_out.
	if _, err := e.pool.Exec(context.Background(),
		`UPDATE orders SET deadline_at = now() - interval '1 hour' WHERE id = $1`, orderID,
	); err != nil {
		t.Fatalf("backdate deadline: %v", err)
	}

	for _, next := range []string{"in_production", "ready", "given_out"} {
		status, resp := e.doJSON(t, "POST", "/barista/orders/"+orderID+"/status", baristaAccess, map[string]string{"status": next})
		if status != http.StatusOK {
			t.Fatalf("переход %s: %d: %v", next, status, resp)
		}
	}

	status, resp := e.doJSON(t, "GET", "/barista/metrics", baristaAccess, nil)
	if status != http.StatusOK {
		t.Fatalf("metrics: %d: %v", status, resp)
	}
	if resp["missed"].(float64) != 1 || resp["on_time"].(float64) != 0 {
		t.Fatalf("metrics: ожидали 0/1, получили %v", resp)
	}
	if resp["on_time_pct"] == "100.0" {
		t.Fatalf("metrics: pct не пересчитан: %v", resp)
	}
}

// readWSMessage читает одно сообщение WS с таймаутом.
func readWSMessage(t *testing.T, conn *websocket.Conn, ctx context.Context) map[string]any {
	t.Helper()
	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, data, err := conn.Read(readCtx)
	if err != nil {
		t.Fatalf("ws read: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("ws unmarshal: %v (%s)", err, data)
	}
	return out
}
