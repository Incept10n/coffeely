-- coffeely: добавки к напиткам и ETA «бариста возьмёт заказ».
--
-- 1) addons — каталог добавок точки (сироп, овсяное молоко, доп. шот).
--    Владелец ведёт его в кабинете, гость выбирает добавки к позиции заказа.
-- 2) order_item_addons — снимок добавки в заказе: название и цена на момент
--    заказа, чтобы правка каталога не переписывала историю.
-- 3) orders.accept_by_at — до какого времени бариста должен взять заказ
--    (настраивается на точке, capacity_config.accept_eta_minutes).

BEGIN;

CREATE TABLE addons (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    point_id     uuid NOT NULL REFERENCES points (id) ON DELETE CASCADE,
    title        text NOT NULL,
    price_minor  integer NOT NULL CHECK (price_minor >= 0),
    prep_seconds integer NOT NULL DEFAULT 0 CHECK (prep_seconds >= 0),
    is_available boolean NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX addons_point_idx ON addons (point_id) WHERE is_available;

CREATE TABLE order_item_addons (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_item_id uuid NOT NULL REFERENCES order_items (id) ON DELETE CASCADE,
    addon_id      uuid REFERENCES addons (id) ON DELETE SET NULL,
    title         text NOT NULL,
    price_minor   integer NOT NULL CHECK (price_minor >= 0),
    prep_seconds  integer NOT NULL DEFAULT 0 CHECK (prep_seconds >= 0)
);

CREATE INDEX order_item_addons_item_idx ON order_item_addons (order_item_id);

-- Старые заказы: ETA не была настроена — считаем, что бариста брал сразу.
ALTER TABLE orders ADD COLUMN accept_by_at timestamptz;
UPDATE orders SET accept_by_at = created_at WHERE accept_by_at IS NULL;
ALTER TABLE orders ALTER COLUMN accept_by_at SET NOT NULL;
ALTER TABLE orders ALTER COLUMN accept_by_at SET DEFAULT now();

COMMIT;
