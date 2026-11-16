-- coffeely: откат 000005 — индексы внутренней админки.

BEGIN;

DROP INDEX IF EXISTS order_items_menu_idx;
DROP INDEX IF EXISTS order_events_event_at_idx;
DROP INDEX IF EXISTS orders_point_created_idx;
DROP INDEX IF EXISTS orders_created_idx;
DROP INDEX IF EXISTS points_owner_idx;

COMMIT;
