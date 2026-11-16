-- Откат: invite-коды возвращаем; точки теряют привязку к владельцу.
ALTER TABLE points DROP COLUMN IF EXISTS owner_id;

CREATE TABLE barista_invite_codes (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code       text NOT NULL UNIQUE,
    point_id   uuid NOT NULL REFERENCES points (id),
    created_by uuid REFERENCES users (id),
    used_by    uuid REFERENCES users (id),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
