-- Откат миграции 000002 (веха M2).

BEGIN;

DROP TABLE IF EXISTS refresh_tokens;

ALTER TABLE barista_invite_codes DROP COLUMN IF EXISTS used_at;

COMMIT;
