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

Каждый входящий запрос пишется в stdout одной JSON-строкой (slog): метод, путь,
статус, байты, длительность, клиент, `X-Request-Id`.

Конфиг через env: `PORT` (8080), `DATABASE_URL`
(дефолт — dev-значение из `deploy/dev/.env.example`; скопируй его в `deploy/dev/.env`,
реальный `.env` в `.gitignore`).

Остановить dev-стек: `make dev-down`. Откатить миграцию: `make migrate-down`.

## Как собрать .apk

Kotlin Multiplatform + Compose Multiplatform, Android-цель (`mobile/`).
Нужны **JDK 21** (AGP 8.13 не работает на JDK 27: падает `jlink` из JdkImageTransform —
локально JDK указывается через `org.gradle.java.home` в `~/.gradle/gradle.properties`)
и Android SDK (`mobile/local.properties` → `sdk.dir=/opt/android-sdk`).

```sh
cd mobile
./gradlew :composeApp:assembleDebug
# mobile/composeApp/build/outputs/apk/debug/composeApp-debug.apk
```

Адрес API зашивается на сборке:

```sh
./gradlew :composeApp:assembleDebug -Pcoffeely.apiBaseUrl=https://coffeely.inceptech.ru/api
```

Без параметра — `http://10.0.2.2:8080` (эмулятор видит хост-машину). CI подставляет прод-адрес.

## Лендинг

`landing/` — статическая страница (Vite + React → nginx), публикует APK и рассказывает
о продукте. Сборка образа:

```sh
cd landing && npm ci && npm run build
docker build -t coffeely-landing .
```

Живёт на https://coffeely.inceptech.ru, APK — https://s3.inceptech.ru/coffeely/coffeely.apk.

## Структура монорепо

| Каталог | Что это | Стек |
|---|---|---|
| `backend/` | API: заказы, точки, меню, движок ёмкости, аутентификация, WebSocket | Go, PostgreSQL |
| `mobile/` | Приложение клиента и бариста | Kotlin Multiplatform + Compose Multiplatform |
| `landing/` | Лендинг + раздача APK | Vite, React, nginx |
| `deploy/` | Compose-стек для VPS (запасной путь), бэкапы | Docker, Caddy |
| `.github/workflows/` | CI: тесты, Docker-образы, сборка APK в S3 | GitHub Actions |

Прод-деплой — GitOps: образы из этого репозитория тянет ArgoCD из
`Incept10n/inceptech` (`apps/coffeely`). CI в кластер не ходит: только build + push
мутабельных тегов `coffeely-backend:backend` и `coffeely-landing:front`.

## Роли

- **client** — выбор кофейни с текущим ожиданием, заказ (окно назначает точка),
  статусы и «мои заказы» (email+пароль, Yandex ID).
- **barista** — очередь заказов с дедлайнами (KDS), фикс промахов (email+пароль, код приглашения к точке).
- Владелец кофейни — отдельный CRM-сайт (другой проект, тот же API), в мобильном приложении его нет.

## Стек-решения (кратко)

- Бэкенд — свой, на Go: полный контроль, оплата хостинга в рублях (VPS РФ), один API на все клиенты.
- Realtime — WebSocket (v1), Centrifugo — при росте.
- Мобильное — KMP + Compose MP: Android первый релиз, iOS в течение полугода.
- Push — FCM, позже RuStore Push.
- Эквайринг — ЮKassa / CloudPayments (после юрлица).

## Статус

- [x] Ресёрч, юнит-экономика, конкурентный анализ (см. базу знаний ai-knowlege, YouGile)
- [x] Зафиксирован стек и структура репо
- [x] Dev-стек (Postgres в docker) и схема БД v1, миграции, health-пробы с проверкой БД
- [x] Бэкенд: аутентификация (email + Yandex ID), роли, коды приглашений бариста
- [x] Бариста-KDS: очередь, дедлайны, realtime
- [x] Клиентский флоу v2: загрузка точки, заказ без выбора окон, «мои заказы», статусы realtime
- [x] Мобильное приложение: клиент и бариста против реального API, сохранение сессии
- [x] Лендинг с публикацией APK
- [x] CI: тесты, Docker-образы в Docker Hub, APK в S3
- [ ] Прод-деплой в кластер (манифесты готовы, ждут применения)
- [ ] Оплата онлайн (M6 — после юрлица)
- [ ] Пилот на 2–3 точках
