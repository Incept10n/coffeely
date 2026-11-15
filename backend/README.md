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
| `ENV` | `dev` | dev / prod |

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
internal/auth/    — (M2) email+пароль (argon2), Yandex ID OAuth2/OIDC, JWT-сессии
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
