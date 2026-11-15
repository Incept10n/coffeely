# backend — API coffeely

Go-бэкенд платформы coffeely.

## Запуск (dev)

```sh
cd backend
go run ./cmd/server
# проверка:
curl http://localhost:8080/healthz
```

Порт задаётся `PORT` (по умолчанию 8080).

## Планируемая структура

```
cmd/server/       — точка входа
internal/config/  — конфигурация (env)
internal/server/  — HTTP-роуты, middleware
internal/auth/    — email+пароль (argon2), Yandex ID OAuth2/OIDC, JWT-сессии
internal/orders/  — заказы, статусы, KDS-очередь
internal/points/  — точки, меню
internal/capacity/— движок слотов ёмкости (5-минутные окна)
migrations/       — SQL-миграции (golang-migrate)
```

## Домены и роли

- Роли: `client`, `barista` (+ будущий `owner` для CRM-сайта).
- Бариста привязывается к точке через код приглашения; видит только заказы своей точки.
- Движок ёмкости v1: конфигурируемое число заказов на 5-минутное окно на точку
  (баристы × заказы/окно); автоподбор по модели кофемашины — v2.
- Realtime-очередь баристы: Centrifugo (v1 — WebSocket).
