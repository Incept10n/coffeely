-- coffeely: схема v1 (веха M1).
-- Ядро продукта (AIK-11): точки, ёмкость (capacity_config), заказы с deadline_at,
-- журнал событий order_events для метрик дедлайнов (AIK-14 §6).

BEGIN;

CREATE EXTENSION IF NOT EXISTS citext;

CREATE TYPE user_role AS ENUM ('client', 'barista', 'owner'); -- owner — под будущий CRM-сайт

CREATE TYPE order_status AS ENUM (
    'created',
    'paid',
    'in_production',
    'ready',
    'given_out',
    'cancelled'
);

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    role          user_role NOT NULL,
    email         citext NOT NULL UNIQUE,
    password_hash text,
    yandex_id     text UNIQUE,
    created_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_has_auth CHECK (
        password_hash IS NOT NULL OR yandex_id IS NOT NULL
    )
);

CREATE TABLE points (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    title           text NOT NULL,
    address         text,
    geo             jsonb NOT NULL DEFAULT '{}'::jsonb, -- {"lat": .., "lon": ..}; PostGIS позже, если понадобится
    timezone        text NOT NULL DEFAULT 'Europe/Moscow',
    capacity_config jsonb NOT NULL DEFAULT '{}'::jsonb, -- слоты/окна/лимиты (движок ёмкости, AIK-11)
    is_active       boolean NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now()
);

-- Многие-к-одному: один бариста — одна точка (PK = user_id).
CREATE TABLE barista_points (
    user_id  uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    point_id uuid NOT NULL REFERENCES points (id),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE barista_invite_codes (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code       text NOT NULL UNIQUE,
    point_id   uuid NOT NULL REFERENCES points (id),
    created_by uuid REFERENCES users (id),
    used_by    uuid REFERENCES users (id),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE menu_items (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    point_id     uuid NOT NULL REFERENCES points (id) ON DELETE CASCADE,
    title        text NOT NULL,
    price_minor  integer NOT NULL CHECK (price_minor >= 0),
    prep_seconds integer NOT NULL CHECK (prep_seconds >= 0),
    is_available boolean NOT NULL DEFAULT true
);

CREATE INDEX menu_items_point_idx ON menu_items (point_id) WHERE is_available;

-- deadline_at вычисляется движком времени (ядро AIK-11):
-- окно выдачи минус сумма prep_seconds напитков заказа.
CREATE TABLE orders (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    point_id          uuid NOT NULL REFERENCES points (id),
    client_id         uuid NOT NULL REFERENCES users (id),
    status            order_status NOT NULL DEFAULT 'created',
    pickup_slot_start timestamptz NOT NULL,
    pickup_slot_end   timestamptz NOT NULL,
    deadline_at       timestamptz NOT NULL,
    total_minor       integer NOT NULL DEFAULT 0 CHECK (total_minor >= 0),
    payment_ref       text,
    created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX orders_point_status_idx ON orders (point_id, status);
CREATE INDEX orders_client_idx ON orders (client_id);

CREATE TABLE order_items (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id     uuid NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    menu_item_id uuid NOT NULL REFERENCES menu_items (id),
    qty          integer NOT NULL CHECK (qty > 0),
    price_minor  integer NOT NULL CHECK (price_minor >= 0) -- цена на момент заказа
);

CREATE INDEX order_items_order_idx ON order_items (order_id);

-- Журнал статусов; из него считаются % дедлайнов и промахи (AIK-14 §6).
CREATE TABLE order_events (
    id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id uuid NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    event    text NOT NULL, -- имя события/статуса; enum появился бы в v2, когда набор устоится
    at       timestamptz NOT NULL DEFAULT now(),
    actor_id uuid REFERENCES users (id)
);

CREATE INDEX order_events_order_idx ON order_events (order_id, at);

COMMIT;
