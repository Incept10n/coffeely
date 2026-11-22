-- Координаты точки для карты (M10). Широта/долгота необязательны: точка
-- без координат остаётся в списке приложения, но не показывается на карте.
-- Историческое поле geo (jsonb) не трогаем — оно нигде не используется.
ALTER TABLE points ADD COLUMN lat double precision;
ALTER TABLE points ADD COLUMN lng double precision;

ALTER TABLE points ADD CONSTRAINT points_lat_range
    CHECK (lat IS NULL OR (lat >= -90 AND lat <= 90));
ALTER TABLE points ADD CONSTRAINT points_lng_range
    CHECK (lng IS NULL OR (lng >= -180 AND lng <= 180));

-- Частичный индекс: карта берёт только точки с координатами. Для десятков
-- точек он не обязателен, но стоит копейки и пригодится при росте.
CREATE INDEX points_coords_idx ON points (lat, lng)
    WHERE lat IS NOT NULL AND lng IS NOT NULL;
