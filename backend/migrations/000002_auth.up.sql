-- coffeely: миграция 000002 (веха M2 — аутентификация).
-- Refresh-токены: храним только SHA-256 хеш; ротация на каждый refresh,
-- повторное использование отозванного токена отзывает все токены пользователя.

BEGIN;

CREATE TABLE refresh_tokens (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash  text NOT NULL UNIQUE,
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    revoked_at  timestamptz,
    replaced_by uuid REFERENCES refresh_tokens (id)
);

CREATE INDEX refresh_tokens_user_idx ON refresh_tokens (user_id);

ALTER TABLE barista_invite_codes ADD COLUMN used_at timestamptz;

COMMIT;
