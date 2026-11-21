-- coffeely: название кофейни у владельца и добавки на уровне товара.
--
-- 1) users.brand_name — как называется кофейня владельца. Точки сохраняют
--    свои названия (адрес/витрина), а бренд один на кабинет.
-- 2) addons.menu_item_id — добавки принадлежат конкретному товару: у капучино
--    свои, у чая может не быть вовсе. Старые добавки точки раскладываются
--    копиями по всем её товарам, чтобы поведение не изменилось молча.

BEGIN;

ALTER TABLE users ADD COLUMN brand_name text NOT NULL DEFAULT '';

ALTER TABLE addons ADD COLUMN menu_item_id uuid REFERENCES menu_items (id) ON DELETE CASCADE;

-- Совместимость: каждая добавка точки становится добавкой каждого её товара.
INSERT INTO addons (menu_item_id, title, price_minor, prep_seconds, is_available)
SELECT mi.id, a.title, a.price_minor, a.prep_seconds, a.is_available
FROM addons a
JOIN menu_items mi ON mi.point_id = a.point_id;

DELETE FROM addons WHERE menu_item_id IS NULL;

ALTER TABLE addons ALTER COLUMN menu_item_id SET NOT NULL;

-- Индекс addons_point_idx удалится вместе с колонкой point_id.
ALTER TABLE addons DROP COLUMN point_id;

CREATE INDEX addons_menu_item_idx ON addons (menu_item_id) WHERE is_available;

COMMIT;
