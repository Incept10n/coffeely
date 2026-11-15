#!/usr/bin/env bash
# Бэкап Postgres coffeely (AIK-13: инфраструктура должна работать без ухода).
# Cron на VPS: 0 3 * * * /srv/coffeely/deploy/backup.sh >> /var/log/coffeely-backup.log 2>&1
#
# Переменные берутся из deploy/.env.prod (лежит рядом со скриптом).
set -euo pipefail

DEPLOY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${DEPLOY_DIR}/.env.prod"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/coffeely}"
KEEP_DAYS="${KEEP_DAYS:-14}"

if [[ ! -f "${ENV_FILE}" ]]; then
    echo "нет ${ENV_FILE}" >&2
    exit 1
fi
# shellcheck disable=SC1090
set -a; source "${ENV_FILE}"; set +a

mkdir -p "${BACKUP_DIR}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT="${BACKUP_DIR}/coffeely-${STAMP}.sql.gz"

# pg_dump через контейнер: на хосте не нужен psql-клиент.
docker compose --env-file "${ENV_FILE}" -f "${DEPLOY_DIR}/docker-compose.prod.yml" \
    exec -T db pg_dump -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" --no-owner \
    | gzip > "${OUT}"

# Проверка целостности архива: пустой/битый бэкап хуже отсутствия.
if [[ ! -s "${OUT}" ]] || ! gzip -t "${OUT}"; then
    echo "БЭКАП ПОВРЕЖДЁН: ${OUT}" >&2
    exit 1
fi

find "${BACKUP_DIR}" -name 'coffeely-*.sql.gz' -mtime +"${KEEP_DAYS}" -delete

echo "ok: ${OUT} ($(du -h "${OUT}" | cut -f1)), храним ${KEEP_DAYS} дней"
