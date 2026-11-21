BEGIN;

ALTER TABLE addons ADD COLUMN point_id uuid REFERENCES points (id) ON DELETE CASCADE;

UPDATE addons a
SET point_id = mi.point_id
FROM menu_items mi
WHERE mi.id = a.menu_item_id;

DELETE FROM addons WHERE point_id IS NULL;

ALTER TABLE addons ALTER COLUMN point_id SET NOT NULL;

-- Индекс addons_menu_item_idx удалится вместе с колонкой menu_item_id.
ALTER TABLE addons DROP COLUMN menu_item_id;

CREATE INDEX addons_point_idx ON addons (point_id) WHERE is_available;

ALTER TABLE users DROP COLUMN brand_name;

COMMIT;
