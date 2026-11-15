# PLAN.md — вехи разработки coffeely

Статус: обновлён 2026-11-15 после создания репозитория.
Источник требований: YouGile `ai-knowlege` (AIK-11, AIK-14, AIK-13, AIK-15).

## Принципы

- Работаем вехами, не по часам. Код пишет агент; пользователь — решения и приёмка.
- Каждая веха заканчивается: зелёной сборкой, коммитом(-ами) в `main`, обновлением
  статуса в этом файле и, если принято бизнес-решение, — записью в `ai-knowlege`.
- Пороговые метрики из AIK-14 §6 (дедлайны, повторные заказы, промахи) должны
  собираться системой с первой версии — это данные для проверки гипотезы.

## Сделано

- [x] Ресёрч, экономика, конкуренты (в `ai-knowlege`)
- [x] Зафиксирован стек: Go-бэкенд (VPS РФ), KMP + Compose MP мобилка, роли client/barista (AIK-15)
- [x] Репозиторий github.com/Incept10n/coffeely (public), монорепо: backend / mobile / deploy
- [x] Go-сервер со health-пробами (`/healthz`, `/readyz`), stdlib, сборка проверена
- [x] Docker установлен и проверен (`hello-world` проходит)
- [x] AGENTS.md — правила для агента и факты окружения

## M1 — Dev-окружение и схема БД (ЗАКРЫТА 2026-11-15)

**Цель:** поднимаемый одной командой dev-стек и первая версия схемы, на которой
строятся M2–M4.

Задачи:
1. `deploy/dev/docker-compose.yml`: postgres:17 (том, healthcheck), и nothing else.
   Переменные — в `deploy/dev/.env.example`, реальный `.env` в `.gitignore`.
2. Миграции `backend/migrations/` (golang-migrate, SQL, up/down). Схема v1:
   - `users` (id uuid, role enum client/barista/owner, email citext unique, password_hash,
     yandex_id, created_at) — role owner резервируем под будущий CRM-сайт;
   - `points` (id, title, address, geo, timezone, capacity_config jsonb — слоты/окна/лимиты,
     is_active);
   - `barista_points` (user_id ↔ point_id, многие-к-одному: один бариста — одна точка) 
     + `barista_invite_codes` (code unique, point_id, created_by, used_by, expires_at);
   - `menu_items` (point_id, title, price_minor, prep_seconds, is_available);
   - `orders` (id, point_id, client_id, status enum created/paid/in_production/ready/
     given_out/cancelled, pickup_slot_start, pickup_slot_end, deadline_at, total_minor,
     payment_ref, created_at) — `deadline_at` вычисляется движком, это ядро AIK-11;
   - `order_items` (order_id, menu_item_id, qty, price_minor);
   - `order_events` (order_id, event, at, actor_id) — журнал статусов, из него считаются
     метрики % дедлайнов и промахов (AIK-14 §6).
3. `backend/internal/config` — конфиг из env (PORT, DATABASE_URL), загрузка в main.
4. Подключение БД: `pgx`/`pgxpool`, graceful shutdown, `readyz` начинает проверять БД.
5. Makefile в `backend/`: `make dev-up`, `make migrate`, `make build`, `make test`.

**Приёмка (выполнено 2026-11-15):** `make dev-up && make migrate && make run` —
`readyz` отвечает `{"status":"ready","db":true}`; миграции подняты с нуля, откат и
повторный подъём проверены; схема в коммите. Примечание: docker 29 требует
отдельного пакета `docker-compose` (compose-плагин), на машине установлен.

## M2 — Аутентификация и роли (ЗАКРЫТА 2026-11-15)

1. Регистрация/вход по email+пароль: argon2id (RFC 9106, 64 MiB), JWT access (15 мин,
   HS256 + jti) + refresh (30 дней, ротация, в БД только SHA-256 хеш; повторное
   использование отозванного токена отзывает все токены пользователя).
2. Yandex ID: `/auth/yandex/start` → `/auth/yandex/callback`, связка по email
   (вход по yandex_id / привязка к email / регистрация). URLs конфигурируемые —
   тесты гоняются против mock-OIDC.
3. Роли: middleware `Authenticate` + `RequireRole`; бариста привязывается к точке
   через `POST /barista/claim-invite` (клиент → бариста атомарно, в транзакции).
4. Коды приглашений: выпуск только owner (`POST /admin/invite-codes`, владелец
   сеется в БД до M7), одноразовость на уровне UPDATE ... WHERE used_by IS NULL,
   TTL (дефолт 7 дней), читаемый Crockford base32.

**Приёмка (выполнено 2026-11-15):** e2e-тесты в `internal/server/e2e_test.go` —
регистрация/вход/refresh+ротация/re-use/выход, Yandex ID на mock-OIDC, claim-invite
(успех, повтор, чужой код, протухший код), 401/403 на чужих ролях. Всё зелёное
против живого Postgres (`coffeely_test`). Smoke-тест на dev-сервере пройден.

## M3 — Бариста-KDS (вход в производство) (ЗАКРЫТА 2026-11-15)

**Ядро продукта: без этого ничего не работает (AIK-11 §4).**

1. API: очередь заказов точки (live), статусы `in_production` / `ready` / `given_out`,
   фикс промаха дедлайна.
2. Realtime v1: WebSocket-канал на точку (позже Centrifugo) — новые заказы и статусы
   толкаются баристе без поллинга.
3. Дедлайны: `deadline_at` = время окна выдачи минус prep_seconds напитков; движок
   считает и отдаёт баристе «№412 — к 8:41, делать сейчас».
4. Метрики: % дедлайнов, соблюдённых вовремя, по точке и по дню.

**Приёмка (выполнено 2026-11-15):** e2e — заказ появляется в очереди в realtime
(WebSocket, `TestRealtimeE2E`), бариста меняет статус, гость видит изменение
вторым API-клиентом (`GET /orders/{id}`), метрика пишется (`TestDeadlineMissE2E`:
промах → missed=1, % пересчитан). Движок дедлайна проверен на точных значениях
(`TestOrderAndQueueE2E`: окно − max prep_seconds).

Реализация: `internal/orders` (store: create/queue/transition/metrics; handlers;
Hub — подписки на точку, `GET /ws/points/{id}`). Создание заказа v1 — статус `paid`
(«оплата в кассе при получении»), проверка ёмкости окна — M4. Сквозные номера
заказов — миграция 000003 (`order_number`).

## M4 — Клиентский флоу (СЛЕДУЮЩАЯ)

1. Список точек (geo), меню точки, окна выдачи на сегодня со свободной ёмкостью
   (движок v1: лимит заказов на 5-минутное окно из `capacity_config`).
2. Создание заказа: выбор окна → проверка ёмкости → заказ → статус для клиента
   с точным временем готовности (не «7–10 минут»).
3. Оплата: v1 «оплата в кассе» (пилот); онлайн-оплата (ЮKassa) — M6.

**Приёмка:** e2e — клиент видит окна, берёт окно, второй клиент на то же окно
получает отказ при исчерпании лимита; статус отражается у баристы и клиента.

## M5 — Мобильное приложение (KMP + Compose MP)

Скелет `mobile/`: gradle-проект, `composeApp` с флоу-заготовками client/barista,
Ktor-клиент против dev-бэкенда, логин-экраны. Для сборки нужен JDK 17+ и Android SDK —
**не установлены, запросить у пользователя установку или сделать это вместе**.
iOS-цель не включать до отдельного решения.

Статус: скелет создан досрочно, 2026-11-15 (по прямому запросу пользователя) —
gradle-проект, тема, логин, заготовки флоу, Ktor-клиент с проверкой `/readyz`.
Осталось от M5: установка Android SDK, первая сборка debug-apk, экранные флоу
под реальными API из M2–M4.

## M6 — Оплата онлайн (ЮKassa / CloudPayments)

После юрлица. Вебхук, идемпотентность, статусы `paid`/`cancelled`.

## M7 — Пилот на 2–3 точках

Deploy на VPS РФ (docker compose, Caddy TLS, бэкапы Postgres, алерты).
Замер порогов AIK-14 §6 на живых данных. До M7 — полевая проверка без кода
(план: AIK-11 §7), её делает пользователь.

## После пилота

- CRM-сайт для владельцев — **отдельный репозиторий** `devops/coffeely-crm`.
- Отчёт по точке (возвращённые заказы, простой, снятое время кассы) — аргумент продаж (AIK-14 §4).
- Движок ёмкости v2: автоподбор по модели кофемашины и числу баристов.

## Журнал

- 2026-11-15 — создан план; M0 закрыта (репо, сервер, Docker).
- 2026-11-15 — M1 закрыта: dev-Postgres, схема v1, миграции, конфиг, pgxpool, Makefile, README (сборка и план сборки .apk).
- 2026-11-15 — скелет mobile/ создан досрочно (запрос пользователя): KMP-проект, тема, логин, заготовки флоу client/barista, Ktor-клиент. Первая сборка apk ждёт установки Android SDK.
- 2026-11-15 — M2 закрыта: argon2id, JWT access+refresh с ротацией и детекцией re-use, Yandex ID (mock-OIDC в тестах), роли через middleware, коды приглашений баристы. e2e-тесты зелёные.
- 2026-11-15 — M3 закрыта: KDS-очередь, переходы статусов с валидацией, движок дедлайна, realtime по WebSocket, метрики % дедлайнов. e2e-тесты зелёные. Следующая веха — M4.
