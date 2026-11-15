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
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"

	"github.com/Incept10n/coffeely/backend/internal/auth"
	"github.com/Incept10n/coffeely/backend/internal/barista"
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

// Тест: коды приглашений и права (403).
func TestInviteCodesE2E(t *testing.T) {
	e := newTestEnv(t)
	e.srv = httptest.NewServer(e.handler)
	t.Cleanup(e.srv.Close)
	ctx := context.Background()

	// seed: точка и владелец
	var pointID, ownerID string
	if err := e.pool.QueryRow(ctx,
		`INSERT INTO points (title, address) VALUES ('Точка тест', 'ул. Тестовая 1') RETURNING id`,
	).Scan(&pointID); err != nil {
		t.Fatalf("seed point: %v", err)
	}
	if err := e.pool.QueryRow(ctx,
		`INSERT INTO users (email, role, password_hash) VALUES ($1, 'owner', $2) RETURNING id`,
		uniqueEmail(), "$argon2id$v=19$m=65536,t=3,p=4$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	).Scan(&ownerID); err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	ownerAccess, err := auth.IssueAccessToken(e.cfg, ownerID, "owner", "owner@test.coffeely")
	if err != nil {
		t.Fatalf("owner token: %v", err)
	}

	// клиент регистрируется
	email := uniqueEmail()
	status, resp := e.doJSON(t, "POST", "/auth/register", "", map[string]string{"email": email, "password": "supersecret1"})
	if status != http.StatusCreated {
		t.Fatalf("register: статус %d", status)
	}
	clientAccess, clientRefresh := tokensFrom(t, resp)

	// клиент не может выпускать коды
	status, _ = e.doJSON(t, "POST", "/admin/invite-codes", clientAccess, map[string]string{"point_id": pointID})
	if status != http.StatusForbidden {
		t.Fatalf("admin/invite-codes клиентом: ждём 403, получили %d", status)
	}
	// без токена тоже
	status, _ = e.doJSON(t, "POST", "/admin/invite-codes", "", map[string]string{"point_id": pointID})
	if status != http.StatusUnauthorized {
		t.Fatalf("admin/invite-codes без токена: ждём 401, получили %d", status)
	}

	// owner выпускает код
	status, resp = e.doJSON(t, "POST", "/admin/invite-codes", ownerAccess, map[string]any{"point_id": pointID, "ttl_hours": 24})
	if status != http.StatusCreated {
		t.Fatalf("invite create: статус %d: %v", status, resp)
	}
	code, _ := resp["code"].(string)
	if code == "" {
		t.Fatalf("invite create: пустой код: %v", resp)
	}

	// неизвестный код
	status, _ = e.doJSON(t, "POST", "/barista/claim-invite", clientAccess, map[string]string{"code": "NOPE1234"})
	if status != http.StatusNotFound {
		t.Fatalf("claim unknown: ждём 404, получили %d", status)
	}

	// клиент использует код → становится баристой точки
	status, resp = e.doJSON(t, "POST", "/barista/claim-invite", clientAccess, map[string]string{"code": code})
	if status != http.StatusOK {
		t.Fatalf("claim: статус %d: %v", status, resp)
	}
	if resp["point_id"] != pointID {
		t.Fatalf("claim: чужая точка: %v", resp)
	}

	// роль обновляется после refresh (роль читается из БД)
	status, resp = e.doJSON(t, "POST", "/auth/refresh", "", map[string]string{"refresh_token": clientRefresh})
	if status != http.StatusOK {
		t.Fatalf("refresh после claim: статус %d: %v", status, resp)
	}
	baristaAccess, _ := tokensFrom(t, resp)
	status, resp = e.doJSON(t, "GET", "/auth/me", baristaAccess, nil)
	if status != http.StatusOK || resp["role"] != "barista" {
		t.Fatalf("me после claim: статус %d, роль %v", status, resp["role"])
	}

	// код одноразовый
	status, _ = e.doJSON(t, "POST", "/barista/claim-invite", baristaAccess, map[string]string{"code": code})
	if status != http.StatusGone {
		t.Fatalf("claim re-use: ждём 410, получили %d", status)
	}

	// протухший код
	expired, err := barista.NewStore(e.pool).CreateInviteCode(ctx, pointID, ownerID, -1*time.Hour)
	if err != nil {
		t.Fatalf("expired code: %v", err)
	}
	status, _ = e.doJSON(t, "POST", "/barista/claim-invite", baristaAccess, map[string]string{"code": expired.Code})
	if status != http.StatusGone {
		t.Fatalf("claim expired: ждём 410, получили %d", status)
	}

	// второй клиент берёт свежий код → тоже бариста (своя точка)
	status, resp = e.doJSON(t, "POST", "/admin/invite-codes", ownerAccess, map[string]any{"point_id": pointID})
	if status != http.StatusCreated {
		t.Fatalf("invite create 2: статус %d", status)
	}
	code2, _ := resp["code"].(string)
	email2 := uniqueEmail()
	_, resp = e.doJSON(t, "POST", "/auth/register", "", map[string]string{"email": email2, "password": "supersecret1"})
	access2, _ := tokensFrom(t, resp)
	status, resp = e.doJSON(t, "POST", "/barista/claim-invite", access2, map[string]string{"code": code2})
	if status != http.StatusOK {
		t.Fatalf("claim 2: статус %d: %v", status, resp)
	}
}
