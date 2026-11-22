package server

// E2E: координаты точки для карты (веха M10). Владелец сохраняет lat/lng
// в кабинете, гость получает их в GET /points. Точка без координат остаётся
// в списке, но без полей lat/lng.

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func pointByID(t *testing.T, pointsAny any, id string) map[string]any {
	t.Helper()
	points, ok := pointsAny.([]any)
	if !ok {
		t.Fatalf("points: не массив: %T", pointsAny)
	}
	for _, raw := range points {
		p, ok := raw.(map[string]any)
		if ok && p["id"] == id {
			return p
		}
	}
	t.Fatalf("точка %s не найдена в %v", id, pointsAny)
	return nil
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestPointCoordsE2E(t *testing.T) {
	e := newTestEnv(t)
	e.srv = httptest.NewServer(e.handler)
	t.Cleanup(e.srv.Close)

	// Владелец на сайте.
	status, resp := e.doJSON(t, "POST", "/owners/register", "",
		map[string]string{"email": uniqueEmail(), "password": "supersecret1"})
	if status != http.StatusCreated {
		t.Fatalf("owners/register: %d: %v", status, resp)
	}
	ownerAccess, _ := tokensFrom(t, resp)

	// Точка с координатами.
	status, resp = e.doJSON(t, "POST", "/admin/points", ownerAccess, map[string]any{
		"title": "Точка на карте", "address": "ул. Ленина, 1",
		"lat": 55.7558, "lng": 37.6173,
		"work_start": "08:00", "work_end": "20:00",
	})
	if status != http.StatusCreated {
		t.Fatalf("создание точки: %d: %v", status, resp)
	}
	pointID, _ := resp["id"].(string)
	if pointID == "" {
		t.Fatalf("нет id: %v", resp)
	}
	if lat, _ := resp["lat"].(float64); !near(lat, 55.7558) {
		t.Fatalf("lat в ответе создания: %v", resp["lat"])
	}
	if lng, _ := resp["lng"].(float64); !near(lng, 37.6173) {
		t.Fatalf("lng в ответе создания: %v", resp["lng"])
	}

	// Владелец видит координаты в кабинете.
	status, resp = e.doJSON(t, "GET", "/admin/points", ownerAccess, nil)
	if status != http.StatusOK {
		t.Fatalf("admin/points: %d: %v", status, resp)
	}
	owned := pointByID(t, resp["points"], pointID)
	if lat, _ := owned["lat"].(float64); !near(lat, 55.7558) {
		t.Fatalf("lat в кабинете: %v", owned["lat"])
	}

	// Гость получает координаты в списке точек.
	client := registerClient(t, e)
	status, resp = e.doJSON(t, "GET", "/points", client, nil)
	if status != http.StatusOK {
		t.Fatalf("points: %d: %v", status, resp)
	}
	visible := pointByID(t, resp["points"], pointID)
	if lat, _ := visible["lat"].(float64); !near(lat, 55.7558) {
		t.Fatalf("lat у гостя: %v", visible["lat"])
	}
	if lng, _ := visible["lng"].(float64); !near(lng, 37.6173) {
		t.Fatalf("lng у гостя: %v", visible["lng"])
	}

	// Владелец двигает пин.
	status, resp = e.doJSON(t, "PATCH", "/admin/points/"+pointID, ownerAccess,
		map[string]any{"lat": 59.9343, "lng": 30.3351})
	if status != http.StatusOK {
		t.Fatalf("PATCH координат: %d: %v", status, resp)
	}
	if lat, _ := resp["lat"].(float64); !near(lat, 59.9343) {
		t.Fatalf("lat после PATCH: %v", resp["lat"])
	}

	// Точка без координат: в списке есть, lat/lng отсутствуют.
	status, resp = e.doJSON(t, "POST", "/admin/points", ownerAccess, map[string]any{
		"title": "Точка без карты", "address": "ул. Без Координат, 2",
		"work_start": "08:00", "work_end": "20:00",
	})
	if status != http.StatusCreated {
		t.Fatalf("создание точки без координат: %d: %v", status, resp)
	}
	noCoordsID, _ := resp["id"].(string)
	status, resp = e.doJSON(t, "GET", "/points", client, nil)
	if status != http.StatusOK {
		t.Fatalf("points после точки без координат: %d", status)
	}
	noCoords := pointByID(t, resp["points"], noCoordsID)
	if _, ok := noCoords["lat"]; ok {
		t.Fatalf("у точки без координат не должно быть lat: %v", noCoords)
	}

	// Половина пары координат — ошибка.
	status, _ = e.doJSON(t, "POST", "/admin/points", ownerAccess, map[string]any{
		"title": "Только широта", "lat": 55.0,
		"work_start": "08:00", "work_end": "20:00",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("lat без lng: ждём 400, получили %d", status)
	}

	// Выход за диапазон — ошибка.
	status, _ = e.doJSON(t, "POST", "/admin/points", ownerAccess, map[string]any{
		"title": "Кривая широта", "lat": 91.0, "lng": 37.0,
		"work_start": "08:00", "work_end": "20:00",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("lat=91: ждём 400, получили %d", status)
	}

	// PATCH одной половины пары — тоже ошибка.
	status, _ = e.doJSON(t, "PATCH", "/admin/points/"+pointID, ownerAccess,
		map[string]any{"lng": 30.0})
	if status != http.StatusBadRequest {
		t.Fatalf("PATCH lng без lat: ждём 400, получили %d", status)
	}
}
