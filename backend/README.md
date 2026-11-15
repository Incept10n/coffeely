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

## API (веха M2)

| Метод | Путь | Доступ | Что делает |
|---|---|---|---|
| POST | `/auth/register` | все | регистрация клиента (email+пароль, argon2id), выдаёт access+refresh |
| POST | `/auth/login` | все | вход, выдаёт access+refresh |
| POST | `/auth/refresh` | все | ротация refresh: старый отзывается, выдаётся новый; повторное использование отзывает всю сессию |
| POST | `/auth/logout` | все | отзывает refresh-токен |
| GET | `/auth/me` | Bearer | текущий пользователь (id, email, роль) |
| GET | `/auth/yandex/start` | все | редирект на Yandex ID |
| GET | `/auth/yandex/callback` | все | вход/регистрация по Yandex ID, связка по email |
| POST | `/barista/claim-invite` | Bearer | одноразовый код → роль barista + привязка к точке |
| POST | `/admin/invite-codes` | Bearer owner | выпуск кода приглашения для точки |

E2E-тесты (`go test ./...`) гоняют все флоу против тестовой БД `coffeely_test`
(пересоздаётся с нуля); Yandex ID — против mock-OIDC. Без доступного Postgres тесты скипаются.

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
