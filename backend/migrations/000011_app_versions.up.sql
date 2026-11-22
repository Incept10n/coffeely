-- Реестр сборок мобильного приложения и политика обновлений.
--
-- Приложение при старте спрашивает GET /app/version: сервер сравнивает его
-- version_code с реестром и отвечает, есть ли новая сборка и обязательна ли она.
-- mandatory = true у сборки означает «со старых версий входить нельзя»:
-- минимально допустимая версия = максимальный version_code среди mandatory.
-- Поэтому «сделать версию блокирующей» — это одна строка в реестре, а не
-- пересборка APK и не правка клиента.
--
-- download_url у APK — публичная ссылка S3 (тот же файл, что отдаёт лендинг).
-- sha256 нужен клиенту, чтобы проверить целостность скачанного файла перед
-- установкой: телефон ставит обновление поверх подписанного тем же ключом.

BEGIN;

CREATE TABLE app_versions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    platform     text NOT NULL CHECK (platform IN ('android', 'ios')),
    version_code integer NOT NULL CHECK (version_code > 0),
    version_name text NOT NULL DEFAULT '',
    download_url text NOT NULL,
    sha256       text NOT NULL DEFAULT '',
    size_bytes   bigint NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    mandatory    boolean NOT NULL DEFAULT false,
    changelog    text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (platform, version_code)
);

-- Политика читается на каждом старте приложения: оба запроса (последняя сборка
-- и минимально допустимая) идут по одному индексу.
CREATE INDEX app_versions_latest_idx ON app_versions (platform, version_code DESC);

COMMIT;
