-- Админская модель (решение 2026-11-16): владельцы кофейен регистрируются на
-- сайте и в личном кабинете создают свои точки и рабочие аккаунты бариста
-- (POST /admin/points, POST /admin/baristas). Роль owner живёт только на сайте:
-- в приложение владельцы не входят (POST /auth/login и Yandex-связка для owner
-- запрещены; кабинет — /owners/*, админка — /admin/*).
ALTER TABLE points ADD COLUMN owner_id uuid REFERENCES users (id);

-- Invite-флоу упразднён: бариста создаёт владелец напрямую, привязка к точке —
-- таблица barista_points (заполняется POST /admin/baristas).
DROP TABLE IF EXISTS barista_invite_codes;
