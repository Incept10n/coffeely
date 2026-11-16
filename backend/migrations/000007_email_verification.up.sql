-- coffeely: подтверждение email после регистрации в приложении.
-- Код хранится только хешем (HMAC-SHA256 с ключом из JWT_SECRET), живёт
-- ограниченное время, попытки считаются: 6 цифр нельзя подбирать бесконечно.
--
-- Существующие пользователи считаются подтверждёнными: иначе после наката
-- миграции все аккаунты разом потеряли бы вход. Владельцы и бариста тоже
-- подтверждены — их email задаёт владелец/сайт, а не сам гость.

BEGIN;

ALTER TABLE users ADD COLUMN email_verified_at timestamptz;

UPDATE users SET email_verified_at = now();

CREATE TABLE email_verification_codes (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    code_hash   text NOT NULL,
    expires_at  timestamptz NOT NULL,
    attempts    integer NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),
    consumed_at timestamptz
);

CREATE INDEX email_verification_codes_user_idx
    ON email_verification_codes (user_id, created_at DESC);

COMMIT;
