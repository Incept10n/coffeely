# AGENTS.md — инструкции для агента в репозитории coffeely

## Что это за проект

**coffeely** (имя всегда пишется так, двойное «ee»; опечатка «cofelly» запрещена) —
платформа предзаказа кофе с управлением ёмкостью точки: баристе — дедлайн производства,
гостю — точное время готовности, кофейне — картина загрузки.

Состав продукта:
- `backend/` — Go-бэкенд (chi/echo, PostgreSQL, миграции golang-migrate; realtime — Centrifugo или WebSocket).
- `mobile/` — приложение клиента и бариста (Kotlin Multiplatform + Compose Multiplatform; Android первым, iOS в течение полугода).
- `landing/` — лендинг (Vite + React → nginx) на https://coffeely.inceptech.ru, раздаёт APK из S3.
- `deploy/` — Docker Compose, VPS РФ, Caddy, TLS (запасной путь; прод — Kubernetes).
- `.github/workflows/ci.yml` — тесты, Docker-образы в Docker Hub, сборка APK в S3.
- CRM для владельцев кофейен и сайт — **отдельный репозиторий** (`devops/coffeely-crm`), тот же API. Здесь его не делать.

## База знаний бизнеса — YouGile `ai-knowlege` (MCP)

Вся бизнес-фактура (проблема, экономика, конкуренты, решения) живёт в YouGile, проект `ai-knowlege`.
При любом вопросе о бизнесе **сначала читать индекс `AIK-2`**, потом нужные записи.

Ключевые записи:
- `AIK-11` — ядро продукта: движок времени и ёмкости точки. Продали не «предзаказ», а расписание производства.
- `AIK-14` — проблема (ёмкость, не очередь) и пороговые условия фальсификации.
- `AIK-13` — соло-режим: минимум операционного ухода, «работает без нас».
- `AIK-15` — зафиксированный стек (этот репо следует ему).
- `AIK-8` — нейминг: coffeely.

Правила работы с базой: одна логическая единица = одна задача; запись в базу — только после показа
пользователю полного текста и его явного «да»; индекс `AIK-2` обновляется тем же ходом.

## Правила работы

- **Код пишет агент.** Не планировать работу по часам — работать вехами (см. `docs/PLAN.md`).
- **Секреты в репо не попадают.** `.env`, ключи, PEM — только в `.gitignore`-закрытых местах.
- Git: ветка `main`, коммиты `feat:/fix:/chore:/docs:`. Аутентификация GitHub по SSH-ключу (аккаунт `Incept10n`).
- Не удалять и не переписывать существующие записи/файлы без разбора — они могут быть работой пользователя.
- Не читать личные заметки SiYuan без явной просьбы пользователя.
- Бизнес-решения, принятые в разговоре, фиксировать в `ai-knowlege` (предложить пользователю), не оставлять только в чате.

## Окружение этой машины

- Arch Linux x86_64, юзер `incept1on` (группы `wheel`, `docker`).
- Go 1.27 (`/usr/bin/go`). JDK 27 установлен (`/usr/lib/jvm/java-27-openjdk`), но
  **AGP 8.13 на нём не собирает apk** (падает `jlink` в JdkImageTransform): для
  `mobile/` нужен JDK 21 — он лежит в `~/.jdks/jdk-21.0.13+9`, путь прописан в
  `~/.gradle/gradle.properties` (`org.gradle.java.home`).
  Android SDK **установлен** в `/opt/android-sdk` (platform 36, build-tools 36, platform-tools, emulator).
  AVD для тестов: `my_avd` (system-image android-36 google_apis x86_64). Запуск без окна:
  `emulator -avd my_avd -no-window -no-audio -no-boot-anim -gpu swiftshader_indirect`
  (GPU → software rendering, окно под Wayland работает нестабильно).
  Инструкция по сборке apk и запуску в эмуляторе — `mobile/README.md`.
- Docker установлен пользователем. Если в сессии `docker` недоступен по правам
  (`permission denied ... /var/run/docker.sock`), обернуть команду в `newgrp docker <<<'<cmd>'` —
  это артефакт старой сессии, в новой сессии docker работает напрямую.
- Postgres в dev — через `deploy/dev/docker-compose.yml` (создать в рамках M1, см. план).

## Прод и CI

- CI (`.github/workflows/ci.yml`) только собирает и пушит: Docker-образы
  `incept1on/coffeely-backend:backend`, `incept1on/coffeely-landing:front` в Docker Hub
  и APK в S3 (`https://s3.inceptech.ru/coffeely/coffeely.apk`). Секреты репозитория:
  `DOCKER_USERNAME`, `DOCKER_PASSWORD`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`.
- В кластер ничего не деплоим из этого репо. ArgoCD (репозиторий `Incept10n/inceptech`,
  `apps/coffeely`) тянет мутабельные теги с `imagePullPolicy: Always`; после нового
  образа нужен `kubectl rollout restart deployment/backend -n coffeely`.
- Секреты прода — OpenBao `secret/coffeely` (`password`, `jwt_secret`) → ExternalSecret
  собирает `DATABASE_URL`, `POSTGRES_PASSWORD`, `JWT_SECRET` в `coffeely-secrets`.
- Домен: `coffeely.inceptech.ru` — лендинг на `/`, API и WebSocket на `/api`
  (nginx срезает префикс перед проксированием в backend);
  TLS — cert-manager, конфиг nginx-LB в репозитории `nginx-inceptech-loadblalancer`.

## Запуск backend (dev)

```sh
cd backend
go run ./cmd/server        # PORT=8080
curl http://localhost:8080/healthz
```

## Текущая позиция и следующие шаги

См. `docs/PLAN.md` (раздел «Следующая веха»). Не начинать новую веху, не закрыв текущую
по её критериям приёмки.
