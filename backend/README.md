# backend — API coffeely

Go-бэкенд платформы coffeely.

## Запуск (dev)

Нужны Go 1.24+ и Docker с compose-плагином.

```sh
cd backend
make dev-up      # Postgres 17 в docker (deploy/dev/.env, см. .env.example)
make migrate     # применить миграции (backend/migrations/, golang-migrate)
make run         # сервер на :8080
```

Health-пробы:

```sh
curl localhost:8080/healthz   # {"status":"ok",...} — процесс жив
curl localhost:8080/readyz    # {"status":"ready","db":true} — и БД отвечает
```

## Миграции

SQL up/down в `backend/migrations/`; схема v1 (веха M1): users (роли
client/barista/owner), points (capacity_config), barista_points,
barista_invite_codes, menu_items, orders (deadline_at — ядро AIK-11),
order_items, order_events (журнал для метрик дедлайнов, AIK-14 §6).

```sh
make migrate        # вверх до конца
make migrate-down   # откатить одну
go run ./cmd/migrate drop   # снести всё (только dev!)
```

## Конфиг (env)

| Переменная | По умолчанию | Смысл |
|---|---|---|
| `PORT` | `8080` | порт HTTP-сервера |
| `DATABASE_URL` | dev-DSN из `deploy/dev/.env.example` | строка подключения Postgres |
| `ENV` | `dev` | dev / prod (в prod обязателен `JWT_SECRET`) |
| `JWT_SECRET` | dev-дефолт | секрет HS256 access-токенов |
| `ACCESS_TOKEN_TTL` | `15m` | срок жизни access-токена |
| `REFRESH_TOKEN_TTL` | `720h` | срок жизни refresh-токена (30 дней) |
| `YANDEX_CLIENT_ID` / `YANDEX_CLIENT_SECRET` | пусто | OAuth2 Yandex ID |
| `YANDEX_REDIRECT_URL` | `http://localhost:8080/auth/yandex/callback` | callback OAuth |

## Логирование

Каждый входящий запрос пишется в stdout одной JSON-строкой (`slog`, middleware
`httpx.LogRequests`): `method`, `path`, `status`, `bytes`, `duration_ms`, `remote`,
`user_agent`, `request_id`. 4xx — WARN, 5xx — ERROR. `X-Request-Id` принимается от
клиента (nginx) или генерируется и возвращается в ответе.

## API

| Метод | Путь | Доступ | Что делает |
|---|---|---|---|
| POST | `/auth/register` | все | регистрация клиента (email+пароль, argon2id), выдаёт access+refresh |
| POST | `/auth/login` | все | вход, выдаёт access+refresh |
| POST | `/auth/refresh` | все | ротация refresh: старый отзывается, выдаётся новый; повторное использование отзывает всю сессию |
| POST | `/auth/logout` | все | отзывает refresh-токен |
| GET | `/auth/me` | Bearer | текущий пользователь (id, email, роль) |
| GET | `/auth/yandex/start` | все | редирект на Yandex ID |
| GET | `/auth/yandex/callback` | все | вход/регистрация по Yandex ID, связка по email |
| GET | `/points` | Bearer | активные точки + загрузка: `load.wait_seconds`, `load.active_orders`, `load.is_open`, `load.opens_at` |
| GET | `/points/{id}/menu` | Bearer | меню точки (цена, `prep_seconds`) |
| POST | `/orders` | Bearer | создать заказ: `{point_id, items[]}`. **Окно не передаётся** — его назначает движок ёмкости |
| GET | `/orders` | Bearer | «мои заказы»: активные сверху, затем история (30) |
| GET | `/orders/{id}` | Bearer | статус заказа (владелец или бариста точки) |
| GET | `/barista/queue` | Bearer barista | KDS-очередь с дедлайнами и срочностью |
| POST | `/barista/orders/{id}/status` | Bearer barista | переход `paid → in_production → ready → given_out` |
| GET | `/barista/metrics?day=YYYY-MM-DD` | Bearer barista | % соблюдённых дедлайнов за день |
| GET | `/ws/points/{id}` | Bearer barista | realtime точки: новые заказы и статусы |
| GET | `/ws/orders` | Bearer | realtime клиента: статусы его заказов |
| POST | `/barista/claim-invite` | Bearer | одноразовый код → роль barista + привязка к точке |
| POST | `/admin/invite-codes` | Bearer owner | выпуск кода приглашения для точки |

### Движок ёмкости (v2, без выбора окон)

Клиент не выбирает «окошки». `POST /orders` назначает окно сам (`points.PlanSlot`):

1. `queue` — сумма `prep_seconds × qty` по активным заказам точки (`paid`, `in_production`);
2. готовность = `сейчас + queue + prep заказа`, округлённая вверх по сетке окон точки;
3. берётся первое окно с свободной ёмкостью (`max_orders_per_window`), окно должно
   начинаться не раньше готовности — бариста получает полный `prep` до начала окна;
4. `deadline_at = начало окна − prep заказа` — это время, которое видит гость;
5. дальше `max_lead` (120 минут) от «сейчас» — отказ `409 point_busy`;
6. гонка двух клиентов на одно окно закрыта `pg_advisory_xact_lock` по точке.

Вне рабочих часов (`work_start`/`work_end` в `capacity_config`, таймзона точки)
окно переносится на следующее открытие.

E2E-тесты (`go test ./...`) гоняют все флоу против тестовой БД `coffeely_test`
(пересоздаётся с нуля); Yandex ID — против mock-OIDC. Без доступного Postgres тесты скипаются.

## Бутстрап точки (dev и пилот)

Первая точка, меню и владелец создаются seed-командой (до появления CRM):

```sh
go run ./cmd/seed \
  -point "coffeely на Ленина" -address "ул. Ленина 1" \
  -email owner@example.com -password "strong-password" \
  -menu "Капучино:25000:180,Латте:27000:150,Американо:18000:90"
```

Формат меню: `Название:цена_в_копейках:prep_секунд`. Команда идемпотентна
(повторный запуск не дублирует точку/юзеров/позиции). Владелец выпускает
invite-коды через `POST /admin/invite-codes`, бариста клеймит точку кодом.

## Сборка

```sh
make build   # bin/server
make test
```

## Структура

```
cmd/server/       — точка входа
cmd/migrate/      — запуск миграций (go run ./cmd/migrate up|down|drop)
internal/config/  — конфигурация (env)
internal/db/      — pgxpool, подключение к БД
internal/server/  — сборка маршрутов + e2e-тесты
internal/auth/    — (M2) argon2id, JWT access+refresh, Yandex ID, middleware ролей
internal/barista/ — (M2) коды приглашений, привязка баристы к точке
internal/httpx/   — JSON-помощники, API-ошибки
internal/orders/  — (M3/M4) заказы, статусы, KDS-очередь
internal/points/  — (M4) точки, меню
internal/capacity/— (M4) движок слотов ёмкости (5-минутные окна)
migrations/       — SQL-миграции (golang-migrate)
```

## Домены и роли

- Роли: `client`, `barista` (+ `owner` зарезервирован под CRM-сайт).
- Бариста привязывается к точке через код приглашения; видит только заказы своей точки.
- Движок ёмкости v1: конфигурируемое число заказов на 5-минутное окно на точку
  (баристы × заказы/окно); автоподбор по модели кофемашины — v2.
- Realtime-очередь баристы: Centrifugo (v1 — WebSocket).
