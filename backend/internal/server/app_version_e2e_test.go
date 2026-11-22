package server

// E2E реестра сборок приложения: публичная проверка обновлений и аварийная
// блокировка версии по секрету админки. Требует Postgres (как остальные e2e),
// без БД тест скипается.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAppVersionE2E(t *testing.T) {
	e := newTestEnv(t)
	e.srv = httptest.NewServer(e.handler)
	t.Cleanup(e.srv.Close)

	send := func(method, path, secret string, body any) (int, map[string]any) {
		t.Helper()
		var reader *strings.Reader
		if body != nil {
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal body: %v", err)
			}
			reader = strings.NewReader(string(raw))
		} else {
			reader = strings.NewReader("")
		}
		req, err := http.NewRequest(method, e.srv.URL+path, reader)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
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

	policy := func(versionCode string) map[string]any {
		t.Helper()
		status, body := send("GET", "/app/version?platform=android&version_code="+versionCode, "", nil)
		if status != http.StatusOK {
			t.Fatalf("app/version %s: статус %d: %v", versionCode, status, body)
		}
		return body
	}

	// Реестр пуст: приложение не блокируется и обновлений не предлагает.
	empty := policy("1874")
	if empty["update_available"] != false || empty["update_required"] != false {
		t.Fatalf("пустой реестр: ожидали без обновлений: %v", empty)
	}
	if empty["min_supported_version_code"] != float64(0) {
		t.Fatalf("пустой реестр: min_supported=%v, ждём 0", empty["min_supported_version_code"])
	}

	release := map[string]any{
		"platform":     "android",
		"version_code": 1880,
		"version_name": "0.1.0+e2e5d63",
		"download_url": "https://s3.inceptech.ru/coffeely/coffeely.apk",
		"sha256":       strings.Repeat("ab", 32),
		"size_bytes":   17_000_000,
		"mandatory":    false,
		"changelog":    "карта точек, поиск по адресу",
	}

	// Публикация без секрета запрещена.
	if status, _ := send("POST", "/internal/admin/app-versions", "", release); status != http.StatusUnauthorized {
		t.Fatalf("publish без секрета: ждём 401, получили %d", status)
	}
	// Мусор в полях отбивается до записи в БД.
	for name, patch := range map[string]map[string]any{
		"плохая платформа": {"platform": "windows"},
		"нулевая версия":   {"version_code": 0},
		"не http-ссылка":   {"download_url": "ftp://s3.inceptech.ru/coffeely.apk"},
		"короткий sha256":  {"sha256": "abc"},
	} {
		bad := map[string]any{}
		for k, v := range release {
			bad[k] = v
		}
		for k, v := range patch {
			bad[k] = v
		}
		if status, _ := send("POST", "/internal/admin/app-versions", "test-admin-secret", bad); status != http.StatusBadRequest {
			t.Fatalf("%s: ждём 400, получили %d", name, status)
		}
	}

	// Обычная (неблокирующая) сборка: старым версиям предлагаем обновиться.
	if status, body := send("POST", "/internal/admin/app-versions", "test-admin-secret", release); status != http.StatusOK {
		t.Fatalf("publish: статус %d: %v", status, body)
	}

	old := policy("1874")
	if old["update_available"] != true || old["update_required"] != false {
		t.Fatalf("1874 при latest=1880: ждём доступное, но не обязательное обновление: %v", old)
	}
	if old["latest_version_code"] != float64(1880) || old["sha256"] != release["sha256"] {
		t.Fatalf("1874: политика не та: %v", old)
	}

	current := policy("1880")
	if current["update_available"] != false || current["update_required"] != false {
		t.Fatalf("1880 (последняя): обновлений быть не должно: %v", current)
	}
	ahead := policy("1900")
	if ahead["update_available"] != false || ahead["update_required"] != false {
		t.Fatalf("1900 (сборка впереди реестра): сервер не должен её трогать: %v", ahead)
	}

	// Блокирующая сборка: 1880 закрывает вход всем, кто ниже.
	mandatoryRelease := map[string]any{}
	for k, v := range release {
		mandatoryRelease[k] = v
	}
	mandatoryRelease["mandatory"] = true
	mandatoryRelease["changelog"] = "исправлен движок окон"
	if status, body := send("POST", "/internal/admin/app-versions", "test-admin-secret", mandatoryRelease); status != http.StatusOK {
		t.Fatalf("publish mandatory: статус %d: %v", status, body)
	}

	blocked := policy("1874")
	if blocked["update_required"] != true || blocked["min_supported_version_code"] != float64(1880) {
		t.Fatalf("1874 при mandatory=1880: ждём обязательное обновление: %v", blocked)
	}
	if blocked["changelog"] != "исправлен движок окон" {
		t.Fatalf("changelog не обновился при повторной публикации: %v", blocked)
	}

	// Аварийное снятие блокировки — без пересборки APK.
	if status, body := send("PATCH", "/internal/admin/app-versions/android/1880", "test-admin-secret",
		map[string]any{"mandatory": false}); status != http.StatusOK {
		t.Fatalf("unset mandatory: статус %d: %v", status, body)
	}
	unblocked := policy("1874")
	if unblocked["update_required"] != false || unblocked["update_available"] != true {
		t.Fatalf("после снятия блокировки 1874 снова может войти: %v", unblocked)
	}

	// Несуществующая сборка и кривой PATCH.
	if status, _ := send("PATCH", "/internal/admin/app-versions/android/9999", "test-admin-secret",
		map[string]any{"mandatory": true}); status != http.StatusNotFound {
		t.Fatalf("PATCH неизвестной версии: ждём 404, получили %d", status)
	}
	if status, _ := send("PATCH", "/internal/admin/app-versions/android/1880", "test-admin-secret",
		map[string]any{}); status != http.StatusBadRequest {
		t.Fatalf("PATCH без mandatory: ждём 400, получили %d", status)
	}

	// Публичный запрос с мусором не должен валить сервер и не должен врать.
	if status, _ := send("GET", "/app/version?platform=windows&version_code=1874", "", nil); status != http.StatusBadRequest {
		t.Fatalf("platform=windows: ждём 400, получили %d", status)
	}
	if status, _ := send("GET", "/app/version?platform=android&version_code=abc", "", nil); status != http.StatusBadRequest {
		t.Fatalf("version_code=abc: ждём 400, получили %d", status)
	}
	if status, _ := send("GET", "/app/version?platform=android", "", nil); status != http.StatusBadRequest {
		t.Fatalf("без version_code: ждём 400, получили %d", status)
	}
}
