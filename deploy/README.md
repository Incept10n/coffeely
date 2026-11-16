# deploy — инфраструктура coffeely

VPS РФ (Timeweb / Selectel / Yandex Cloud), оплата в рублях.

Артефакты prod-деплоя готовы (веха M7), ждут VPS и домен:

| Файл | Что это |
|---|---|
| `docker-compose.prod.yml` | Postgres + миграции + backend + Caddy |
| `Caddyfile` | TLS автоматически (Let's Encrypt), проксирование API и WebSocket |
| `.env.prod.example` | шаблон prod-переменных (реальный `.env.prod` в `.gitignore`) |
| `backup.sh` | бэкап Postgres через `pg_dump` с проверкой целостности и ротацией |
| `dev/` | локальное dev-окружение (только Postgres) |

## Деплой на VPS (пошагово)

Требования: Ubuntu/Debian, 2 vCPU / 2 ГБ RAM, Docker с compose-плагином,
DNS A-запись домена → IP сервера.

```sh
# 1. Код
git clone git@github.com:Incept10n/coffeely.git /srv/coffeely
cd /srv/coffeely/deploy

# 2. Конфиг (сгенерировать секреты)
cp .env.prod.example .env.prod && chmod 600 .env.prod
#   POSTGRES_PASSWORD:  openssl rand -base64 32
#   JWT_SECRET:         openssl rand -base64 48
#   ADMIN_CRM_SECRET:   openssl rand -base64 48  (вход в /admin)
#   DOMAIN + YANDEX_*:  заполнить

# 3. Поднять
docker compose --env-file .env.prod -f docker-compose.prod.yml up -d --build

# 4. Проверка
curl https://api.<домен>/healthz   # {"status":"ok",...}
curl https://api.<домен>/readyz    # {"status":"ready","db":true}
```

Миграции применяются сервисом `migrate` при каждом `up -d` (идемпотентно).

## Бэкапы (AIK-13: работает без нас)

```sh
# cron: каждый день в 3:00
0 3 * * * /srv/coffeely/deploy/backup.sh >> /var/log/coffeely-backup.log 2>&1
```

Бэкапы: `/var/backups/coffeely/coffeely-<UTC>.sql.gz`, хранение 14 дней,
пустой/битый архив = ошибка (не перезаписывается «тихо»).

Восстановление:

```sh
gunzip -c /var/backups/coffeely/coffeely-<stamp>.sql.gz | \
  docker compose --env-file .env.prod -f docker-compose.prod.yml exec -T db \
  psql -U coffeely -d coffeely
```

## Что ещё нужно для пилота

- Алерты (uptime-мониторинг на `/readyz` + телеграм-бот) — после выбора сервиса.
- Ограничение доступа к VPS (SSH-ключи, fail2ban), обновления безопасности.
- Первый owner-аккаунт для выпуска invite-кодов создаётся SQL-вставкой до появления CRM.
