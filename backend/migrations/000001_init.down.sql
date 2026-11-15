-- Откат схемы v1 (веха M1).

BEGIN;

DROP TABLE IF EXISTS order_events;
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS menu_items;
DROP TABLE IF EXISTS barista_invite_codes;
DROP TABLE IF EXISTS barista_points;
DROP TABLE IF EXISTS points;
DROP TABLE IF EXISTS users;

DROP TYPE IF EXISTS order_status;
DROP TYPE IF EXISTS user_role;

COMMIT;
