-- coffeely: миграция 000003 (веха M3 — бариста-KDS).
-- Сквозной номер заказа для KDS («№412 — к 8:41»).

BEGIN;

CREATE SEQUENCE orders_order_number_seq;

ALTER TABLE orders
    ADD COLUMN order_number bigint NOT NULL DEFAULT nextval('orders_order_number_seq');

CREATE UNIQUE INDEX orders_order_number_idx ON orders (order_number);

COMMIT;
