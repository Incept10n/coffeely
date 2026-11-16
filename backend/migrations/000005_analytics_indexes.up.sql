-- coffeely: миграция 000005 — индексы для внутренней админки (/admin).
-- Аналитика читает всю платформу: заказы по времени, дедлайны по событиям,
-- точки по владельцу. Без этих индексов сводка деградирует с ростом данных.

BEGIN;

CREATE INDEX IF NOT EXISTS points_owner_idx       ON points (owner_id);
CREATE INDEX IF NOT EXISTS orders_created_idx     ON orders (created_at);
CREATE INDEX IF NOT EXISTS orders_point_created_idx ON orders (point_id, created_at);
CREATE INDEX IF NOT EXISTS order_events_event_at_idx ON order_events (event, at);
CREATE INDEX IF NOT EXISTS order_items_menu_idx   ON order_items (menu_item_id);

COMMIT;
