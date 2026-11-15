# coffeely

Платформа предзаказа кофе с управлением ёмкостью точки: баристе — дедлайн производства,
гостю — точное время готовности, кофейне — картина загрузки.

## Как запустить (dev)

Нужны: Go 1.24+, Docker с compose-плагином (`docker compose version`).

```sh
cd backend
make dev-up      # поднимает Postgres 17 (deploy/dev/, том, healthcheck)
make migrate     # применяет SQL-миграции из backend/migrations/
make run         # сервер на :8080
```

Проверки: `curl localhost:8080/healthz` (жив ли процесс) и
`curl localhost:8080/readyz` (жив ли + видит ли БД — `{"status":"ready","db":true}`).

Конфиг через env: `PORT` (8080), `DATABASE_URL`
(дефолт — dev-значение из `deploy/dev/.env.example`; скопируй его в `deploy/dev/.env`,
реальный `.env` в `.gitignore`).

Остановить dev-стек: `make dev-down`. Откатить миграцию: `make migrate-down`.

## Как собрать .apk (мобильное приложение)

Мобильное приложение — Kotlin Multiplatform + Compose Multiplatform (`mobile/`).
Скелет приложения подключается вехой M5 — **сейчас каталог `mobile/` пуст**,
собирать пока нечего. Вот как это будет делаться (и что нужно поставить заранее):

1. Установить JDK 17+ (`sudo pacman -S jdk17-openjdk`) и Android SDK
   (командная строка: `sdkmanager "platform-tools" "platforms;android-34" "build-tools;34.0.0"`,
   или через Android Studio).
2. `mobile/local.properties` (не в git): `sdk.dir=/путь/к/Android/Sdk`.
3. Сборка отладочного apk:

```sh
cd mobile
./gradlew :composeApp:assembleDebug
# результат: mobile/composeApp/build/outputs/apk/debug/composeApp-debug.apk
```

4. Релизный apk/aab: `./gradlew :composeApp:assembleRelease` / `:bundleRelease`
   (нужен подписывающий ключ — `*.keystore` в `.gitignore`, в репо не попадает).

iOS-цель пока не включаем (решение зафиксировано в `docs/PLAN.md`).

## Структура монорепо

| Каталог | Что это | Стек |
|---|---|---|
| `backend/` | API-бэкенд: заказы, точки, меню, слоты ёмкости, аутентификация | Go, PostgreSQL |
| `mobile/` | Приложение клиента и бариста | Kotlin Multiplatform + Compose Multiplatform |
| `deploy/` | Docker Compose, конфиги VPS, TLS | Docker, Caddy |

## Роли

- **client** — заказ кофе, выбор окна выдачи, статусы заказа (email+пароль, Yandex ID).
- **barista** — очередь заказов с дедлайнами (KDS), фикс промахов (email+пароль, код приглашения к точке).
- Владелец кофейни — отдельный CRM-сайт (другой проект, тот же API), в мобильном приложении его нет.

## Стек-решения (кратко)

- Бэкенд — свой, на Go: полный контроль, оплата хостинга в рублях (VPS РФ), один API на все клиенты.
- Realtime — Centrifugo (v1 — WebSocket руками).
- Мобильное — KMP + Compose MP: Android первый релиз, iOS в течение полугода.
- Push — FCM, позже RuStore Push.
- Эквайринг — ЮKassa / CloudPayments (после юрлица).

## Статус

- [x] Ресёрч, юнит-экономика, конкурентный анализ (см. базу знаний ai-knowlege, YouGile)
- [x] Зафиксирован стек и структура репо
- [x] Dev-стек (Postgres в docker) и схема БД v1, миграции, health-пробы с проверкой БД
- [x] Бэкенд: аутентификация (email + Yandex ID), роли, коды приглашений бариста
- [ ] Бариста-KDS (первая веха: вход в производство)
- [ ] Клиентский флоу: точки, меню, окна выдачи, заказ
- [ ] Оплата
- [ ] Мобильное приложение (скелет — M5, см. `docs/PLAN.md`)
- [ ] Пилот на 2–3 точках
