-- Откат миграции 000003 (веха M3).

BEGIN;

DROP INDEX IF EXISTS orders_order_number_idx;
ALTER TABLE orders DROP COLUMN IF EXISTS order_number;
DROP SEQUENCE IF EXISTS orders_order_number_seq;

COMMIT;
