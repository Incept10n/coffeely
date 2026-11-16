package server

// E2E-тесты аутентификации (веха M2). Требуют запущенного Postgres:
// используется отдельная тестовая БД (coffeely_test), схема пересоздаётся с нуля.
// Без доступной БД тесты скипаются (go test ./... работает и без docker).

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/lib/pq"

	"github.com/Incept10n/coffeely/backend/internal/config"
)

const (
	testDSN   = "postgres://coffeely:coffeely@localhost:5432/coffeely_test?sslmode=disable"
	adminDSN  = "postgres://coffeely:coffeely@localhost:5432/postgres?sslmode=disable"
	mockEmail = "yandex.user@example.com"
)

type testEnv struct {
	pool    *pgxpool.Pool
	handler http.Handler
	cfg     config.Config
	mock    *httptest.Server
	srv     *httptest.Server
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	ctx := context.Background()

	// Доступность БД: без неё — скип (CI/локально без docker).
	admin, err := sql.Open("postgres", adminDSN)
	if err != nil {
		t.Skipf("тестовая БД недоступна: %v", err)
	}
	if err := admin.PingContext(ctx); err != nil {
		admin.Close()
		t.Skipf("тестовая БД недоступна: %v", err)
	}
	// Пересоздаём тестовую БД с нуля: чистая схема гарантированно.
	if _, err := admin.ExecContext(ctx, "DROP DATABASE IF EXISTS coffeely_test WITH (FORCE)"); err != nil {
		admin.Close()
		t.Fatalf("drop тестовой БД: %v", err)
	}
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE coffeely_test"); err != nil {
		admin.Close()
		t.Fatalf("создание тестовой БД: %v", err)
	}
	admin.Close()

	m, err := migrate.New("file://../../migrations", testDSN)
	if err != nil {
		t.Fatalf("migrate init: %v", err)
	}
	if err := m.Up(); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil || dbErr != nil {
			t.Logf("migrate close: %v %v", srcErr, dbErr)
		}
	}()

	pool, err := pgxpool.New(ctx, testDSN)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}
	t.Cleanup(pool.Close)

	// Mock Yandex ID (OIDC) — URLs из конфига смотрят на него.
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"access_token":"mock-yandex-token"}`))
		case "/info":
			_, _ = w.Write([]byte(fmt.Sprintf(`{"id":"ya-user-1","default_email":%q}`, mockEmail)))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mock.Close)

	cfg := config.Config{
		Port:               "0",
		DatabaseURL:        testDSN,
		JWTSecret:          fmt.Sprintf("test-secret-%d", time.Now().UnixNano()),
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    30 * 24 * time.Hour,
		YandexClientID:     "test-client-id",
		YandexClientSecret: "test-client-secret",
		YandexRedirectURL:  "http://localhost:8080/auth/yandex/callback",
		YandexAuthURL:      mock.URL + "/authorize",
		YandexTokenURL:     mock.URL + "/token",
		YandexInfoURL:      mock.URL + "/info",
		AdminCRMSecret:     "test-admin-secret",
	}

	return &testEnv{
		pool:    pool,
		handler: New(pool, cfg),
		cfg:     cfg,
		mock:    mock,
	}
}

// doJSON выполняет запрос против тестового сервера; body может быть nil.
func (e *testEnv) doJSON(t *testing.T, method, path, bearer string, body any) (int, map[string]any) {
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
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
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

func tokensFrom(t *testing.T, resp map[string]any) (access, refresh string) {
	t.Helper()
	toks, ok := resp["tokens"].(map[string]any)
	if !ok {
		t.Fatalf("нет tokens в ответе: %v", resp)
	}
	return toks["access_token"].(string), toks["refresh_token"].(string)
}

func uniqueEmail() string {
	return fmt.Sprintf("user-%d@test.coffeely", time.Now().UnixNano())
}

// Тест: регистрация → вход → me → refresh с ротацией → реакция на re-use.
func TestAuthE2E(t *testing.T) {
	e := newTestEnv(t)
	e.srv = httptest.NewServer(e.handler)
	t.Cleanup(e.srv.Close)

	email := uniqueEmail()

	// register
	status, resp := e.doJSON(t, "POST", "/auth/register", "", map[string]string{"email": email, "password": "supersecret1"})
	if status != http.StatusCreated {
		t.Fatalf("register: статус %d: %v", status, resp)
	}
	access, refresh := tokensFrom(t, resp)
	if user, _ := resp["user"].(map[string]any); user["role"] != "client" {
		t.Fatalf("register: роль не client: %v", resp["user"])
	}

	// дубликат email
	if status, _ := e.doJSON(t, "POST", "/auth/register", "", map[string]string{"email": email, "password": "supersecret1"}); status != http.StatusConflict {
		t.Fatalf("register duplicate: ждём 409, получили %d", status)
	}

	// неверный пароль
	if status, _ := e.doJSON(t, "POST", "/auth/login", "", map[string]string{"email": email, "password": "wrong-password"}); status != http.StatusUnauthorized {
		t.Fatalf("login wrong password: ждём 401, получили %d", status)
	}

	// верный вход
	status, resp = e.doJSON(t, "POST", "/auth/login", "", map[string]string{"email": email, "password": "supersecret1"})
	if status != http.StatusOK {
		t.Fatalf("login: статус %d: %v", status, resp)
	}
	access2, _ := tokensFrom(t, resp)
	if access2 == access {
		t.Fatal("login выдал тот же access-токен")
	}

	// /auth/me
	status, resp = e.doJSON(t, "GET", "/auth/me", access2, nil)
	if status != http.StatusOK {
		t.Fatalf("me: статус %d: %v", status, resp)
	}
	if resp["email"] != email {
		t.Fatalf("me: чужой email: %v", resp)
	}

	// /auth/me без токена и с мусором
	if status, _ := e.doJSON(t, "GET", "/auth/me", "", nil); status != http.StatusUnauthorized {
		t.Fatalf("me без токена: ждём 401, получили %d", status)
	}
	if status, _ := e.doJSON(t, "GET", "/auth/me", "garbage.token.here", nil); status != http.StatusUnauthorized {
		t.Fatalf("me с мусорным токеном: ждём 401, получили %d", status)
	}

	// refresh: ротация
	status, resp = e.doJSON(t, "POST", "/auth/refresh", "", map[string]string{"refresh_token": refresh})
	if status != http.StatusOK {
		t.Fatalf("refresh: статус %d: %v", status, resp)
	}
	access3, refresh3 := tokensFrom(t, resp)
	if refresh3 == refresh || access3 == access2 {
		t.Fatal("refresh не ротировал токены")
	}

	// re-use старого refresh → 401 и отзыв всей сессии
	if status, _ := e.doJSON(t, "POST", "/auth/refresh", "", map[string]string{"refresh_token": refresh}); status != http.StatusUnauthorized {
		t.Fatalf("refresh re-use: ждём 401, получили %d", status)
	}
	// новый refresh тоже сожжён
	if status, _ := e.doJSON(t, "POST", "/auth/refresh", "", map[string]string{"refresh_token": refresh3}); status != http.StatusUnauthorized {
		t.Fatalf("refresh после re-use: ждём 401, получили %d", status)
	}
}

// Тест: флоу Yandex ID против mock-OIDC.
func TestYandexE2E(t *testing.T) {
	e := newTestEnv(t)
	e.srv = httptest.NewServer(e.handler)
	t.Cleanup(e.srv.Close)

	// start → редирект на mock с одноразовым state (не следуем за редиректом)
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := noRedirect.Get(e.srv.URL + "/auth/yandex/start")
	if err != nil {
		t.Fatalf("yandex start: %v", err)
	}
	location, err := url.Parse(resp.Header.Get("Location"))
	resp.Body.Close()
	if err != nil || location.Host == "" {
		t.Fatalf("yandex start: нет Location: %q, %v", resp.Header.Get("Location"), err)
	}
	if !strings.HasPrefix(location.String(), e.mock.URL) {
		t.Fatalf("yandex start: редирект не на mock: %s", location.String())
	}
	state := location.Query().Get("state")
	if state == "" {
		t.Fatal("yandex start: нет state")
	}

	// callback → пользователь создан, токены выданы
	status, body := e.doJSON(t, "GET", "/auth/yandex/callback?code=mock-code&state="+state+"&format=json", "", nil)
	if status != http.StatusOK {
		t.Fatalf("yandex callback: статус %d: %v", status, body)
	}
	if user, _ := body["user"].(map[string]any); user["email"] != mockEmail {
		t.Fatalf("yandex callback: email не совпал: %v", body["user"])
	}
	_, _ = tokensFrom(t, body)

	// state одноразовый: повторный callback отклоняется
	status, _ = e.doJSON(t, "GET", "/auth/yandex/callback?code=mock-code&state="+state+"&format=json", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("yandex state re-use: ждём 401, получили %d", status)
	}

	// повторный вход тем же yandex_id → тот же пользователь
	resp2, err := noRedirect.Get(e.srv.URL + "/auth/yandex/start")
	if err != nil {
		t.Fatalf("yandex start 2: %v", err)
	}
	loc2, _ := url.Parse(resp2.Header.Get("Location"))
	resp2.Body.Close()
	status, body2 := e.doJSON(t, "GET", "/auth/yandex/callback?code=mock-code&state="+loc2.Query().Get("state")+"&format=json", "", nil)
	if status != http.StatusOK {
		t.Fatalf("yandex callback 2: статус %d: %v", status, body2)
	}
	if id1, id2 := body["user"].(map[string]any)["id"], body2["user"].(map[string]any)["id"]; id1 != id2 {
		t.Fatalf("yandex: создан второй пользователь вместо входа: %v vs %v", id1, id2)
	}
}

// Тест: бариста назначается флагом в БД (invite-коды упразднены вместе с ролью
// owner): роль подхватывается при refresh (читается из БД), KDS-очередь доступна.
func TestBaristaFlagE2E(t *testing.T) {
	e := newTestEnv(t)
	e.srv = httptest.NewServer(e.handler)
	t.Cleanup(e.srv.Close)
	ctx := context.Background()

	// точка для привязки баристы
	var pointID string
	if err := e.pool.QueryRow(ctx,
		`INSERT INTO points (title, address) VALUES ('Точка тест', 'ул. Тестовая 1') RETURNING id`,
	).Scan(&pointID); err != nil {
		t.Fatalf("seed point: %v", err)
	}

	// клиент регистрируется
	email := uniqueEmail()
	status, resp := e.doJSON(t, "POST", "/auth/register", "", map[string]string{"email": email, "password": "supersecret1"})
	if status != http.StatusCreated {
		t.Fatalf("register: статус %d", status)
	}
	clientAccess, clientRefresh := tokensFrom(t, resp)

	// до флага очередь недоступна
	status, _ = e.doJSON(t, "GET", "/barista/queue", clientAccess, nil)
	if status != http.StatusForbidden {
		t.Fatalf("queue клиентом: ждём 403, получили %d", status)
	}

	// флаг в БД: роль + привязка к точке (то, что раньше делал claim-invite)
	if _, err := e.pool.Exec(ctx,
		`UPDATE users SET role = 'barista' WHERE email = $1`, email,
	); err != nil {
		t.Fatalf("flag barista: %v", err)
	}
	if _, err := e.pool.Exec(ctx,
		`INSERT INTO barista_points (user_id, point_id)
		 SELECT u.id, $2 FROM users u WHERE u.email = $1`, email, pointID,
	); err != nil {
		t.Fatalf("bind point: %v", err)
	}

	// роль обновляется после refresh (роль читается из БД)
	status, resp = e.doJSON(t, "POST", "/auth/refresh", "", map[string]string{"refresh_token": clientRefresh})
	if status != http.StatusOK {
		t.Fatalf("refresh после флага: статус %d: %v", status, resp)
	}
	baristaAccess, _ := tokensFrom(t, resp)
	status, resp = e.doJSON(t, "GET", "/auth/me", baristaAccess, nil)
	if status != http.StatusOK || resp["role"] != "barista" {
		t.Fatalf("me после флага: статус %d, роль %v", status, resp["role"])
	}

	// очередь доступна
	status, resp = e.doJSON(t, "GET", "/barista/queue", baristaAccess, nil)
	if status != http.StatusOK {
		t.Fatalf("queue бариста: статус %d: %v", status, resp)
	}
}

// Тест: кабинет владельца на сайте. Владелец регистрируется через
// /owners/register; вход в приложение ему запрещён (роль owner живёт только
// на сайте); в кабинете создаёт точку с меню и рабочего баристу, который
// затем входит в приложение и видит очередь.
func TestOwnerAdminE2E(t *testing.T) {
	e := newTestEnv(t)
	e.srv = httptest.NewServer(e.handler)
	t.Cleanup(e.srv.Close)

	// владелец регистрируется на сайте
	email := uniqueEmail()
	status, resp := e.doJSON(t, "POST", "/owners/register", "", map[string]string{"email": email, "password": "supersecret1"})
	if status != http.StatusCreated {
		t.Fatalf("owners/register: статус %d: %v", status, resp)
	}
	if role, _ := resp["user"].(map[string]any)["role"].(string); role != "owner" {
		t.Fatalf("owners/register: роль %v", resp["user"])
	}
	ownerAccess, _ := tokensFrom(t, resp)

	// вход владельца в приложение запрещён
	status, _ = e.doJSON(t, "POST", "/auth/login", "", map[string]string{"email": email, "password": "supersecret1"})
	if status != http.StatusForbidden {
		t.Fatalf("auth/login владельцем: ждём 403, получили %d", status)
	}
	// вход владельца в кабинет — ок
	status, _ = e.doJSON(t, "POST", "/owners/login", "", map[string]string{"email": email, "password": "supersecret1"})
	if status != http.StatusOK {
		t.Fatalf("owners/login: статус %d", status)
	}

	// клиент и аноним в админку не допускаются
	client := registerClient(t, e)
	status, _ = e.doJSON(t, "GET", "/admin/points", client, nil)
	if status != http.StatusForbidden {
		t.Fatalf("admin/points клиентом: ждём 403, получили %d", status)
	}
	status, _ = e.doJSON(t, "GET", "/admin/points", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("admin/points без токена: ждём 401, получили %d", status)
	}

	// владелец создаёт точку с меню
	status, resp = e.doJSON(t, "POST", "/admin/points", ownerAccess, map[string]any{
		"title": "Точка владельца", "address": "ул. Тестовая 1",
		"window_minutes": 5, "max_orders_per_window": 3,
		"work_start": "08:00", "work_end": "20:00",
		"menu": []map[string]any{{"title": "Капучино", "price_minor": 25000, "prep_seconds": 180}},
	})
	if status != http.StatusCreated {
		t.Fatalf("admin/points: статус %d: %v", status, resp)
	}
	pointID, _ := resp["id"].(string)
	if pointID == "" {
		t.Fatalf("admin/points: нет id: %v", resp)
	}
	status, resp = e.doJSON(t, "GET", "/admin/points", ownerAccess, nil)
	if status != http.StatusOK || len(resp["points"].([]any)) != 1 {
		t.Fatalf("admin/points list: %d: %v", status, resp)
	}

	// владелец создаёт баристу на свою точку
	bEmail := uniqueEmail()
	status, resp = e.doJSON(t, "POST", "/admin/baristas", ownerAccess, map[string]string{"email": bEmail, "password": "supersecret2", "point_id": pointID})
	if status != http.StatusCreated {
		t.Fatalf("admin/baristas: статус %d: %v", status, resp)
	}
	baristaID, _ := resp["id"].(string)
	if baristaID == "" || resp["password"] != "supersecret2" {
		t.Fatalf("admin/baristas: пароль не вернулся: %v", resp)
	}

	// бариста входит в приложение (разрешено) и видит очередь
	status, resp = e.doJSON(t, "POST", "/auth/login", "", map[string]string{"email": bEmail, "password": "supersecret2"})
	if status != http.StatusOK {
		t.Fatalf("auth/login бариста: статус %d: %v", status, resp)
	}
	if role, _ := resp["user"].(map[string]any)["role"].(string); role != "barista" {
		t.Fatalf("auth/login бариста: роль %v", resp["user"])
	}
	baristaAccess, _ := tokensFrom(t, resp)
	status, resp = e.doJSON(t, "GET", "/barista/queue", baristaAccess, nil)
	if status != http.StatusOK {
		t.Fatalf("queue бариста: статус %d: %v", status, resp)
	}

	// пароль баристы виден владельцу и после перезагрузки страницы
	status, resp = e.doJSON(t, "GET", "/admin/baristas", ownerAccess, nil)
	if status != http.StatusOK {
		t.Fatalf("admin/baristas list: %d: %v", status, resp)
	}
	bList, _ := resp["baristas"].([]any)
	if len(bList) != 1 || bList[0].(map[string]any)["password"] != "supersecret2" {
		t.Fatalf("admin/baristas list: пароль не виден: %v", resp)
	}

	// владелец редактирует аккаунт баристы: email, пароль и точка
	newEmail := uniqueEmail()
	status, resp = e.doJSON(t, "PATCH", "/admin/baristas/"+baristaID, ownerAccess, map[string]any{
		"email": newEmail, "password": "newsecret55", "point_id": pointID,
	})
	if status != http.StatusOK {
		t.Fatalf("admin/baristas patch: %d: %v", status, resp)
	}
	if resp["email"] != newEmail || resp["password"] != "newsecret55" {
		t.Fatalf("admin/baristas patch: не применилось: %v", resp)
	}
	status, _ = e.doJSON(t, "POST", "/auth/login", "", map[string]string{"email": bEmail, "password": "supersecret2"})
	if status != http.StatusUnauthorized {
		t.Fatalf("старый пароль после смены: ждём 401, получили %d", status)
	}
	status, resp = e.doJSON(t, "POST", "/auth/login", "", map[string]string{"email": newEmail, "password": "newsecret55"})
	if status != http.StatusOK {
		t.Fatalf("вход с новым паролем: %d: %v", status, resp)
	}

	// владелец меняет информацию о точке
	status, resp = e.doJSON(t, "PATCH", "/admin/points/"+pointID, ownerAccess, map[string]any{
		"title": "Точка переименована", "address": "ул. Новая 2", "is_active": true,
		"window_minutes": 10, "max_orders_per_window": 5,
		"work_start": "09:00", "work_end": "21:00",
	})
	if status != http.StatusOK {
		t.Fatalf("admin/points patch: %d: %v", status, resp)
	}
	if resp["title"] != "Точка переименована" || resp["address"] != "ул. Новая 2" {
		t.Fatalf("admin/points patch: не применилось: %v", resp)
	}
	capacity, _ := resp["capacity_config"].(map[string]any)
	if capacity["window_minutes"] != float64(10) || capacity["max_orders_per_window"] != float64(5) ||
		capacity["work_start"] != "09:00" || capacity["work_end"] != "21:00" {
		t.Fatalf("admin/points patch: ёмкость не применилась: %v", capacity)
	}

	// владелец видит и правит товары точки
	status, resp = e.doJSON(t, "GET", "/admin/points/"+pointID+"/menu", ownerAccess, nil)
	if status != http.StatusOK {
		t.Fatalf("admin menu: %d: %v", status, resp)
	}
	menu, _ := resp["items"].([]any)
	if len(menu) != 1 {
		t.Fatalf("admin menu: ждём 1 позицию: %v", resp)
	}
	itemID, _ := menu[0].(map[string]any)["id"].(string)
	status, resp = e.doJSON(t, "PATCH", "/admin/menu/"+itemID, ownerAccess, map[string]any{
		"title": "Флэт уайт", "price_minor": 30000, "prep_seconds": 200,
	})
	if status != http.StatusOK || resp["title"] != "Флэт уайт" || resp["price_minor"] != float64(30000) {
		t.Fatalf("admin menu patch: %d: %v", status, resp)
	}
	status, resp = e.doJSON(t, "POST", "/admin/points/"+pointID+"/menu", ownerAccess, map[string]any{
		"title": "Американо", "price_minor": 15000, "prep_seconds": 90,
	})
	if status != http.StatusCreated {
		t.Fatalf("admin menu create: %d: %v", status, resp)
	}
	newItemID, _ := resp["id"].(string)
	status, resp = e.doJSON(t, "PATCH", "/admin/menu/"+newItemID, ownerAccess, map[string]any{"is_available": false})
	if status != http.StatusOK || resp["is_available"] != false {
		t.Fatalf("admin menu hide: %d: %v", status, resp)
	}
	status, resp = e.doJSON(t, "GET", "/admin/points/"+pointID+"/menu", ownerAccess, nil)
	if status != http.StatusOK {
		t.Fatalf("admin menu после правок: %d: %v", status, resp)
	}
	if menu, _ = resp["items"].([]any); len(menu) != 2 {
		t.Fatalf("admin menu после правок: ждём 2 позиции: %v", resp)
	}

	// другой владелец не может привязать баристу к чужой точке
	email2 := uniqueEmail()
	_, resp = e.doJSON(t, "POST", "/owners/register", "", map[string]string{"email": email2, "password": "supersecret3"})
	owner2, _ := tokensFrom(t, resp)
	status, _ = e.doJSON(t, "POST", "/admin/baristas", owner2, map[string]string{"email": uniqueEmail(), "password": "supersecret4", "point_id": pointID})
	if status != http.StatusForbidden {
		t.Fatalf("чужая точка: ждём 403, получили %d", status)
	}
	status, _ = e.doJSON(t, "PATCH", "/admin/points/"+pointID, owner2, map[string]any{"title": "чужая"})
	if status != http.StatusNotFound {
		t.Fatalf("чужая точка patch: ждём 404, получили %d", status)
	}
	status, _ = e.doJSON(t, "PATCH", "/admin/menu/"+itemID, owner2, map[string]any{"title": "чужое"})
	if status != http.StatusNotFound {
		t.Fatalf("чужое меню patch: ждём 404, получили %d", status)
	}
	status, _ = e.doJSON(t, "PATCH", "/admin/baristas/"+baristaID, owner2, map[string]any{"password": "supersecret6"})
	if status != http.StatusNotFound {
		t.Fatalf("чужой бариста patch: ждём 404, получили %d", status)
	}
}
