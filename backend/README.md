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

SQL up/down в `backend/migrations/`; users (роли client/barista; owner —
только для сайта, в приложение не входит), points (capacity_config, owner_id —
владелец точки), barista_points (привязка баристы к точке), menu_items,
orders (deadline_at — ядро AIK-11), order_items, order_events (журнал для
метрик дедлайнов, AIK-14 §6). Invite-коды удалены в 000004. 000005 — индексы
под аналитику внутренней админки (/admin).

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
| `ADMIN_CRM_SECRET` | пусто | строка входа во внутреннюю админку `/admin`; пусто — админка выключена (503) |
| `MAIL_EWS_URL` | `https://mail.inceptech.ru/EWS/Exchange.asmx` | EWS-эндпоинт Exchange |
| `MAIL_USERNAME` | `coffeely@inceptech.ru` | ящик для NTLM-авторизации в Exchange |
| `MAIL_PASSWORD` | пусто | пароль ящика; **в prod обязателен**; пусто — подтверждение email выключено (dev) |
| `MAIL_FROM` | `coffeely@inceptech.ru` | адрес отправителя в письме |
| `MAIL_FROM_NAME` | `coffeely` | имя отправителя |
| `MAIL_INSECURE_SKIP_VERIFY` | `true` | не проверять TLS-сертификат Exchange (внутренний сертификат; `false` — проверять) |
| `MAIL_TIMEOUT` | `10s` | таймаут отправки письма |
| `VERIFICATION_CODE_TTL` | `15m` | срок жизни кода подтверждения email |
| `VERIFICATION_RESEND_PERIOD` | `60s` | минимальный интервал повторной отправки кода |
| `VERIFICATION_MAX_ATTEMPTS` | `5` | попыток ввода кода до запроса нового |

## Логирование

Каждый входящий запрос пишется в stdout одной JSON-строкой (`slog`, middleware
`httpx.LogRequests`): `method`, `path`, `status`, `bytes`, `duration_ms`, `remote`,
`user_agent`, `request_id`. 4xx — WARN, 5xx — ERROR. `X-Request-Id` принимается от
клиента (nginx) или генерируется и возвращается в ответе.

## API

| Метод | Путь | Доступ | Что делает |
|---|---|---|---|
| POST | `/auth/register` | все | регистрация клиента (email+пароль, argon2id); с почтой — `verification_required: true` без токенов, на email уходит код |
| POST | `/auth/verify-email` | все | `{email, code}` → подтверждает email и выдаёт access+refresh |
| POST | `/auth/resend-verification` | все | повторное письмо с кодом (не чаще `VERIFICATION_RESEND_PERIOD`) |
| POST | `/auth/login` | все | вход, выдаёт access+refresh; до подтверждения email — 403 `email_not_verified` |
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
| POST | `/owners/register` | все | регистрация владельца кофейни на сайте (только веб) |
| POST | `/owners/login` | все | вход владельца в кабинет на сайте |
| POST | `/admin/points` | Bearer owner | создать точку (+меню) владельца |
| GET | `/admin/points` | Bearer owner | точки владельца |
| PATCH | `/admin/points/{id}` | Bearer owner | изменить точку: название, адрес, активность, ёмкость |
| GET | `/admin/points/{id}/menu` | Bearer owner | меню точки, включая скрытые позиции |
| POST | `/admin/points/{id}/menu` | Bearer owner | добавить товар в меню точки |
| PATCH | `/admin/menu/{id}` | Bearer owner | изменить товар: название, цена, prep, видимость |
| POST | `/admin/baristas` | Bearer owner | создать рабочий аккаунт баристы на свою точку |
| GET | `/admin/baristas` | Bearer owner | список бариста по точкам владельца; пароль виден владельцу |
| PATCH | `/admin/baristas/{id}` | Bearer owner | изменить баристу: email, пароль, точка (§ см. ниже) |
| POST | `/internal/admin/login` | `X-Admin-Secret` / тело `{secret}` | проверка строки доступа внутренней админки (limiter: 20 попыток / 15 мин на IP) |
| GET | `/internal/admin/analytics?days=30` | `X-Admin-Secret` | сводная аналитика платформы для создателей: деньги, заказы, дедлайны, точки, владельцы, бариста, активация |

Каналы разведены: `/auth/*` — приложение (клиент и бариста), `/owners/*` и
`/admin/*` — сайт (только владелец), `/internal/admin/*` — внутренняя админка
создателей (общий секрет, без пользователей и ролей). Владелец не входит в
приложение (`/auth/login` для owner отвечает 403).

### Подтверждение email

`/auth/register` создаёт клиента, генерирует 6-значный код и отправляет его
письмом через Exchange Web Services (EWS, NTLM). В БД лежит только HMAC-SHA256
хеш кода; попытки считаются, срок и интервал повторной отправки — в конфиге.
Токены регистрация не выдаёт: приложение показывает экран ввода кода и вызывает
`/auth/verify-email`. `/auth/login` до подтверждения отвечает 403
`email_not_verified`. Владельцы и бариста подтверждены сразу: их email задаёт
сайт/владелец, а не гость. Если `MAIL_PASSWORD` пуст (dev), подтверждение
выключено и регистрация работает по-старому — сразу с токенами.

В Kubernetes поду backend нужен маршрут до Exchange: `hostAliases`
`mail.inceptech.ru → 192.168.50.10` (см. `apps/coffeely/backend.yaml` в
репозитории `inceptech`).

Пароли бариста хранятся двумя способами: argon2id-хеш в `users.password_hash`
(проверка входа) и обратимая AES-256-GCM копия в `users.password_enc` — чтобы
владелец видел пароль в кабинете после перезагрузки страницы и мог его сменить.
Ключ выводится из `JWT_SECRET` через HMAC-SHA256 (метка `coffeely/user-password/v1`);
при смене `JWT_SECRET` старые копии перестают читаться — owner задаёт пароль
заново через `PATCH /admin/baristas/{id}`. Смена пароля отзывает refresh-токены
баристы.

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

Точка и меню создаются seed-командой (dev-данные, точки без владельца) либо в
личном кабинете на сайте — основной путь для прода:

```sh
go run ./cmd/seed \
  -point "coffeely на Ленина" -address "ул. Ленина 1" \
  -menu "Капучино:25000:180,Латте:27000:150,Американо:18000:90"
```

Формат меню: `Название:цена_в_копейках:prep_секунд`. Команда идемпотентна
(повторный запуск не дублирует точку/позиции). Пользователей seed не создаёт.

Прод-путь: владелец регистрируется на `https://coffeely.inceptech.ru/owner/register`
(бесплатно), в кабинете `/owner` видит тарифы, добавляет свои точки и создаёт
рабочие аккаунты бариста — бариста входит с ними в приложение и попадает в KDS.
Аварийный путь (без кабинета) — флаг в БД:

```sql
UPDATE users SET role = 'barista' WHERE email = '...';
INSERT INTO barista_points (user_id, point_id)
  SELECT u.id, p.id FROM users u, points p
  WHERE u.email = '...' AND p.title = '...';
```

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
internal/auth/    — argon2id, JWT access+refresh, Yandex ID, middleware ролей
internal/admin/   — кабинет владельца: точки, меню, рабочие аккаунты бариста
internal/securebox/ — обратимое шифрование паролей бариста (AES-256-GCM)
internal/superadmin/ — внутренняя админка создателей (/admin): вход по секрету, аналитика
internal/httpx/   — JSON-помощники, API-ошибки
internal/orders/  — (M3/M4) заказы, статусы, KDS-очередь
internal/points/  — (M4) точки, меню
internal/capacity/— (M4) движок слотов ёмкости (5-минутные окна)
migrations/       — SQL-миграции (golang-migrate)
```

## Домены и роли

- `client` — гость: регистрируется и входит в приложение, заказывает кофе.
- `barista` — рабочий аккаунт, который создаёт владелец в кабинете на сайте
  (или флагом в БД — аварийный путь); входит в приложение, видит KDS своей точки.
- `owner` — владелец кофейни: существует только на сайте (кабинет `/owner`:
  тарифы, свои точки, создание бариста). В приложение не входит.
- Бариста привязан к точке через `barista_points`; видит только заказы своей точки.
- Движок ёмкости v1: конфигурируемое число заказов на 5-минутное окно на точку
  (баристы × заказы/окно); автоподбор по модели кофемашины — v2.
- Realtime-очередь баристы: Centrifugo (v1 — WebSocket).
