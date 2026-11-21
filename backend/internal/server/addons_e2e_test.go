package server

// E2E-тесты добавок к напиткам и ETA «бариста возьмёт заказ» (миграция 000008).

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAddonsAndAcceptEtaE2E(t *testing.T) {
	e := newTestEnv(t)
	e.srv = httptest.NewServer(e.handler)
	t.Cleanup(e.srv.Close)

	// Владелец и точка: часы работы почти круглосуточные, чтобы тест не зависел
	// от времени прогона; перерыв 12:00–12:30 проверяем в юнит-тестах движка.
	ownerEmail := uniqueEmail()
	status, resp := e.doJSON(t, "POST", "/owners/register", "",
		map[string]string{"email": ownerEmail, "password": "supersecret1"})
	if status != http.StatusCreated {
		t.Fatalf("owners/register: %d: %v", status, resp)
	}
	ownerAccess, _ := tokensFrom(t, resp)

	status, resp = e.doJSON(t, "POST", "/admin/points", ownerAccess, map[string]any{
		"title": "Точка с добавками", "address": "ул. Сиропная 1",
		"work_start": "00:00", "work_end": "23:59",
		"accept_eta_minutes": 7,
		"breaks":             []map[string]string{{"start": "03:00", "end": "03:10"}},
		"menu": []map[string]any{
			{"title": "Капучино", "price_minor": 25000, "prep_seconds": 180},
		},
	})
	if status != http.StatusCreated {
		t.Fatalf("admin/points: %d: %v", status, resp)
	}
	pointID, _ := resp["id"].(string)
	if pointID == "" {
		t.Fatalf("нет id точки: %v", resp)
	}
	capacity, _ := resp["capacity_config"].(map[string]any)
	if capacity["accept_eta_minutes"] != float64(7) {
		t.Fatalf("accept_eta_minutes не сохранилась: %v", capacity)
	}
	if breaks, _ := capacity["breaks"].([]any); len(breaks) != 1 {
		t.Fatalf("breaks не сохранились: %v", capacity)
	}

	// Меню и добавка.
	status, resp = e.doJSON(t, "GET", "/admin/points/"+pointID+"/menu", ownerAccess, nil)
	if status != http.StatusOK {
		t.Fatalf("admin menu: %d: %v", status, resp)
	}
	items, _ := resp["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("admin menu: ждём 1 товар: %v", resp)
	}
	menuID, _ := items[0].(map[string]any)["id"].(string)

	status, resp = e.doJSON(t, "POST", "/admin/menu/"+menuID+"/addons", ownerAccess, map[string]any{
		"title": "Сироп карамель", "price_minor": 5000, "prep_seconds": 0,
	})
	if status != http.StatusCreated {
		t.Fatalf("admin addons: %d: %v", status, resp)
	}
	addonID, _ := resp["id"].(string)
	if addonID == "" || resp["title"] != "Сироп карамель" || resp["menu_item_id"] != menuID {
		t.Fatalf("admin addons: не создалась: %v", resp)
	}

	// Бренд кофейни — один на кабинет, точки сохраняют свои названия.
	status, resp = e.doJSON(t, "PATCH", "/admin/profile", ownerAccess,
		map[string]any{"brand_name": "Сиропная кофейня"})
	if status != http.StatusOK || resp["brand_name"] != "Сиропная кофейня" {
		t.Fatalf("admin profile: %d: %v", status, resp)
	}

	// Второй товар без добавок: наборы у товаров независимы.
	status, _ = e.doJSON(t, "POST", "/admin/points/"+pointID+"/menu", ownerAccess, map[string]any{
		"title": "Чай", "price_minor": 12000, "prep_seconds": 60,
	})
	if status != http.StatusCreated {
		t.Fatalf("admin menu tea: %d", status)
	}

	// Гость видит добавку только у капучино; у чая добавок нет.
	clientAccess := registerClient(t, e)
	status, resp = e.doJSON(t, "GET", "/points/"+pointID+"/menu", clientAccess, nil)
	if status != http.StatusOK {
		t.Fatalf("client menu: %d: %v", status, resp)
	}
	menu, _ := resp["menu"].([]any)
	addonsByTitle := map[string][]any{}
	for _, raw := range menu {
		it, _ := raw.(map[string]any)
		title, _ := it["title"].(string)
		list, _ := it["addons"].([]any)
		addonsByTitle[title] = list
	}
	if list := addonsByTitle["Капучино"]; len(list) != 1 ||
		list[0].(map[string]any)["title"] != "Сироп карамель" {
		t.Fatalf("client menu: добавка не видна у капучино: %v", resp)
	}
	if list := addonsByTitle["Чай"]; len(list) != 0 {
		t.Fatalf("client menu: у чая не должно быть добавок: %v", resp)
	}

	// Клиент видит бренд кофейни рядом с точкой.
	status, resp = e.doJSON(t, "GET", "/points", clientAccess, nil)
	if status != http.StatusOK {
		t.Fatalf("client points: %d: %v", status, resp)
	}
	foundBrand := false
	list, _ := resp["points"].([]any)
	for _, raw := range list {
		p, _ := raw.(map[string]any)
		if p["id"] == pointID && p["brand_name"] == "Сиропная кофейня" {
			foundBrand = true
		}
	}
	if !foundBrand {
		t.Fatalf("client points: бренд не виден: %v", resp)
	}

	// Заказ с добавкой: цена добавки входит в итог, ETA баристы заполнена.
	status, resp = e.doJSON(t, "POST", "/orders", clientAccess, map[string]any{
		"point_id": pointID,
		"items": []map[string]any{
			{"menu_item_id": menuID, "qty": 2, "addons": []string{addonID}},
		},
	})
	if status != http.StatusCreated {
		t.Fatalf("create order: %d: %v", status, resp)
	}
	if got := resp["total_minor"].(float64); got != float64((25000+5000)*2) {
		t.Fatalf("total с добавкой: %v, ждём %d", got, (25000+5000)*2)
	}
	acceptBy, _ := resp["accept_by_at"].(string)
	if acceptBy == "" {
		t.Fatalf("accept_by_at пуст: %v", resp)
	}
	if ts, err := time.Parse(time.RFC3339, acceptBy); err != nil || ts.Before(time.Now().Add(5*time.Minute)) {
		t.Fatalf("accept_by_at: %q (%v), ждём ~через 7 минут", acceptBy, err)
	}
	if resp["accept_in_seconds"].(float64) <= 0 {
		t.Fatalf("accept_in_seconds: %v", resp["accept_in_seconds"])
	}
	orderItems, _ := resp["items"].([]any)
	if len(orderItems) != 1 {
		t.Fatalf("items заказа: %v", resp)
	}
	orderAddons, _ := orderItems[0].(map[string]any)["addons"].([]any)
	if len(orderAddons) != 1 || orderAddons[0].(map[string]any)["title"] != "Сироп карамель" {
		t.Fatalf("добавка не попала в заказ: %v", orderItems[0])
	}

	// Скрытая добавка исчезает из меню гостя, но не из истории заказа.
	status, resp = e.doJSON(t, "PATCH", "/admin/addons/"+addonID, ownerAccess, map[string]any{"is_available": false})
	if status != http.StatusOK || resp["is_available"] != false {
		t.Fatalf("hide addon: %d: %v", status, resp)
	}
	status, resp = e.doJSON(t, "GET", "/points/"+pointID+"/menu", clientAccess, nil)
	if status != http.StatusOK {
		t.Fatalf("client menu после скрытия: %d: %v", status, resp)
	}
	menu, _ = resp["menu"].([]any)
	for _, raw := range menu {
		it, _ := raw.(map[string]any)
		if it["title"] == "Капучино" {
			if list, _ := it["addons"].([]any); len(list) != 0 {
				t.Fatalf("скрытая добавка видна гостю: %v", resp)
			}
		}
	}
}
