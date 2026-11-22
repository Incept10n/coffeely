// Package admin — кабинет владельца на сайте: свои точки, меню и рабочие
// аккаунты бариста. Роль owner существует только на сайте (в приложение
// владельцы не входят), поэтому вся админка живёт в отдельных эндпоинтах /admin/*.
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Incept10n/coffeely/backend/internal/httpx"
)

var (
	ErrForeignPoint    = httpx.NewError(403, "point_foreign", "точка принадлежит другому владельцу")
	ErrPointNotFound   = httpx.NewError(404, "point_not_found", "точка не найдена")
	ErrBadCapacity     = httpx.NewError(400, "bad_capacity", "некорректные настройки ёмкости")
	ErrBadCoords       = httpx.NewError(400, "bad_coords", "некорректные координаты точки")
	ErrEmailTaken      = httpx.NewError(409, "email_taken", "email занят")
	ErrBaristaNotFound = httpx.NewError(404, "barista_not_found", "бариста не найден")
	ErrMenuItemMissing = httpx.NewError(404, "menu_item_not_found", "позиция меню не найдена")
	ErrAddonMissing    = httpx.NewError(404, "addon_not_found", "добавка не найдена")
)

// Point — точка кофейни владельца.
type Point struct {
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Address  string          `json:"address"`
	Lat      *float64        `json:"lat,omitempty"`
	Lng      *float64        `json:"lng,omitempty"`
	Capacity json.RawMessage `json:"capacity_config"`
	IsActive bool            `json:"is_active"`
}

// Barista — рабочий аккаунт, созданный владельцем в кабинете.
//
// Password — расшифрованная обратимая копия (users.password_enc), чтобы
// владелец видел пароль в кабинете и после перезагрузки страницы. Пустая
// строка — аккаунт создан до миграции 000006 либо ключ сменился: пароль
// задаётся заново через редактирование.
type Barista struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	PointID     string `json:"point_id"`
	PointTitle  string `json:"point_title"`
	CreatedAt   string `json:"created_at"`
	Password    string `json:"password,omitempty"`
	PasswordEnc []byte `json:"-"`
}

// MenuItem — позиция меню точки. Владелец видит и скрытые позиции: их можно
// вернуть в продажу редактированием. Addons — добавки именно этого товара.
type MenuItem struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	PriceMinor  int64   `json:"price_minor"`
	PrepSeconds int     `json:"prep_seconds"`
	IsAvailable bool    `json:"is_available"`
	Addons      []Addon `json:"addons"`
}

// Addon — добавка к конкретному напитку (сироп, овсяное молоко, доп. шот).
// Наборы у товаров независимы, у части товаров добавок может не быть вовсе.
type Addon struct {
	ID          string `json:"id"`
	MenuItemID  string `json:"menu_item_id"`
	Title       string `json:"title"`
	PriceMinor  int64  `json:"price_minor"`
	PrepSeconds int    `json:"prep_seconds"`
	IsAvailable bool   `json:"is_available"`
}

// Profile — настройки владельца, не привязанные к точке.
type Profile struct {
	BrandName string `json:"brand_name"`
}

// Break — перерыв в работе точки ("HH:MM" локального времени точки).
type Break struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// Store — SQL для точек, меню и бариста владельца.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// CreatePoint — точка + меню одной транзакцией. lat/lng необязательны:
// без них точка не появится на карте, но останется в списке приложения.
func (s *Store) CreatePoint(ctx context.Context, ownerID, title, address string, lat, lng *float64, capacity json.RawMessage, menu []MenuItem) (Point, error) {
	var p Point
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return p, fmt.Errorf("admin: point tx: %w", err)
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `
		INSERT INTO points (title, address, lat, lng, capacity_config, owner_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, title, COALESCE(address, ''), lat, lng, capacity_config, is_active`,
		title, address, lat, lng, capacity, ownerID,
	).Scan(&p.ID, &p.Title, &p.Address, &p.Lat, &p.Lng, &p.Capacity, &p.IsActive)
	if err != nil {
		return p, fmt.Errorf("admin: point insert: %w", err)
	}
	for _, mi := range menu {
		if _, err := tx.Exec(ctx, `
			INSERT INTO menu_items (point_id, title, price_minor, prep_seconds)
			VALUES ($1, $2, $3, $4)`,
			p.ID, mi.Title, mi.PriceMinor, mi.PrepSeconds,
		); err != nil {
			return p, fmt.Errorf("admin: menu insert: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return p, fmt.Errorf("admin: point commit: %w", err)
	}
	return p, nil
}

// ListPoints — точки владельца.
func (s *Store) ListPoints(ctx context.Context, ownerID string) ([]Point, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, title, COALESCE(address, ''), lat, lng, capacity_config, is_active
		FROM points WHERE owner_id = $1
		ORDER BY created_at`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("admin: points list: %w", err)
	}
	defer rows.Close()
	points := []Point{}
	for rows.Next() {
		var p Point
		if err := rows.Scan(&p.ID, &p.Title, &p.Address, &p.Lat, &p.Lng, &p.Capacity, &p.IsActive); err != nil {
			return nil, fmt.Errorf("admin: points scan: %w", err)
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// PointPatch — частичное изменение точки: nil-поля не трогаем.
type PointPatch struct {
	Title              *string
	Address            *string
	Lat                *float64
	Lng                *float64
	IsActive           *bool
	WindowMinutes      *int
	MaxOrdersPerWindow *int
	WorkStart          *string
	WorkEnd            *string
	Breaks             *[]Break
	AcceptETAMinutes   *int
}

// UpdatePoint — меняет название, адрес, активность и параметры ёмкости точки.
// Неизвестные ключи capacity_config сохраняются: конфиг правится поверх.
func (s *Store) UpdatePoint(ctx context.Context, ownerID, pointID string, patch PointPatch) (Point, error) {
	var p Point
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return p, fmt.Errorf("admin: point update tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var capacityRaw []byte
	err = tx.QueryRow(ctx, `
		SELECT id, title, COALESCE(address, ''), lat, lng, capacity_config, is_active
		FROM points WHERE id = $1 AND owner_id = $2
		FOR UPDATE`, pointID, ownerID,
	).Scan(&p.ID, &p.Title, &p.Address, &p.Lat, &p.Lng, &capacityRaw, &p.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrPointNotFound
	}
	if err != nil {
		return p, fmt.Errorf("admin: point update load: %w", err)
	}

	if patch.Title != nil {
		p.Title = *patch.Title
	}
	if patch.Address != nil {
		p.Address = *patch.Address
	}
	if patch.Lat != nil && patch.Lng != nil {
		p.Lat = patch.Lat
		p.Lng = patch.Lng
	}
	if patch.IsActive != nil {
		p.IsActive = *patch.IsActive
	}

	capacity := map[string]any{}
	if len(capacityRaw) > 0 {
		if err := json.Unmarshal(capacityRaw, &capacity); err != nil {
			return p, fmt.Errorf("admin: capacity json: %w", err)
		}
	}
	if patch.WindowMinutes != nil {
		capacity["window_minutes"] = *patch.WindowMinutes
	}
	if patch.MaxOrdersPerWindow != nil {
		capacity["max_orders_per_window"] = *patch.MaxOrdersPerWindow
	}
	if patch.WorkStart != nil {
		capacity["work_start"] = *patch.WorkStart
	}
	if patch.WorkEnd != nil {
		capacity["work_end"] = *patch.WorkEnd
	}
	if patch.Breaks != nil {
		capacity["breaks"] = *patch.Breaks
	}
	if patch.AcceptETAMinutes != nil {
		capacity["accept_eta_minutes"] = *patch.AcceptETAMinutes
	}
	if p.Capacity, err = json.Marshal(capacity); err != nil {
		return p, fmt.Errorf("admin: capacity marshal: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE points
		SET title = $2, address = $3, lat = $4, lng = $5, capacity_config = $6, is_active = $7
		WHERE id = $1`,
		p.ID, p.Title, p.Address, p.Lat, p.Lng, p.Capacity, p.IsActive,
	); err != nil {
		return p, fmt.Errorf("admin: point update: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return p, fmt.Errorf("admin: point update commit: %w", err)
	}
	return p, nil
}

// PointOwnedBy — точка принадлежит владельцу? (перед привязкой баристы и меню)
func (s *Store) PointOwnedBy(ctx context.Context, ownerID, pointID string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM points WHERE id = $1 AND owner_id = $2)`,
		pointID, ownerID,
	).Scan(&ok)
	return ok, err
}

// CreateBarista — рабочий аккаунт баристы + привязка к точке владельца (tx).
// passwordEnc — обратимая копия пароля для показа владельцу в кабинете.
func (s *Store) CreateBarista(ctx context.Context, email, passwordHash string, passwordEnc []byte, pointID string) (Barista, error) {
	var b Barista
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return b, fmt.Errorf("admin: barista tx: %w", err)
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, role, password_hash, password_enc, email_verified_at)
		VALUES ($1, 'barista', $2, $3, now())
		RETURNING id`, email, passwordHash, passwordEnc,
	).Scan(&b.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return b, ErrEmailTaken
		}
		return b, fmt.Errorf("admin: barista insert: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO barista_points (user_id, point_id) VALUES ($1, $2)`,
		b.ID, pointID,
	); err != nil {
		return b, fmt.Errorf("admin: barista bind: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT title FROM points WHERE id = $1`, pointID,
	).Scan(&b.PointTitle); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return b, ErrForeignPoint
		}
		return b, fmt.Errorf("admin: barista point: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT created_at::text FROM barista_points WHERE user_id = $1`, b.ID,
	).Scan(&b.CreatedAt); err != nil {
		return b, fmt.Errorf("admin: barista created: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return b, fmt.Errorf("admin: barista commit: %w", err)
	}
	b.Email, b.PointID = email, pointID
	return b, nil
}

// ListBaristas — все рабочие аккаунты всех точек владельца. Обратимая копия
// пароля возвращается как есть; расшифровывает её хендлер.
func (s *Store) ListBaristas(ctx context.Context, ownerID string) ([]Barista, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.email, bp.point_id, p.title, bp.created_at::text, u.password_enc
		FROM barista_points bp
		JOIN users u ON u.id = bp.user_id
		JOIN points p ON p.id = bp.point_id
		WHERE p.owner_id = $1
		ORDER BY bp.created_at DESC`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("admin: baristas list: %w", err)
	}
	defer rows.Close()
	list := []Barista{}
	for rows.Next() {
		var b Barista
		if err := rows.Scan(&b.ID, &b.Email, &b.PointID, &b.PointTitle, &b.CreatedAt, &b.PasswordEnc); err != nil {
			return nil, fmt.Errorf("admin: baristas scan: %w", err)
		}
		list = append(list, b)
	}
	return list, rows.Err()
}

// BaristaPatch — частичное изменение рабочего аккаунта.
type BaristaPatch struct {
	Email        *string
	PasswordHash *string
	PasswordEnc  []byte
	PointID      *string
}

// UpdateBarista — меняет email, пароль и/или точку баристы. Пароль меняется
// только вместе с хешем и шифрокопией. Смена пароля отзывает refresh-токены,
// чтобы старые сессии не жили со старым паролем.
func (s *Store) UpdateBarista(ctx context.Context, ownerID, baristaID string, patch BaristaPatch) (Barista, error) {
	var b Barista
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return b, fmt.Errorf("admin: barista update tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var owned bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM barista_points bp
			JOIN users u ON u.id = bp.user_id
			JOIN points p ON p.id = bp.point_id
			WHERE bp.user_id = $1 AND u.role = 'barista' AND p.owner_id = $2
		)`, baristaID, ownerID,
	).Scan(&owned); err != nil {
		return b, fmt.Errorf("admin: barista owner check: %w", err)
	}
	if !owned {
		return b, ErrBaristaNotFound
	}

	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET email = COALESCE($2, email),
		    password_hash = COALESCE($3, password_hash),
		    password_enc = COALESCE($4, password_enc)
		WHERE id = $1`,
		baristaID, patch.Email, patch.PasswordHash, patch.PasswordEnc,
	); err != nil {
		if isUniqueViolation(err) {
			return b, ErrEmailTaken
		}
		return b, fmt.Errorf("admin: barista update: %w", err)
	}
	if patch.PointID != nil {
		if _, err := tx.Exec(ctx,
			`UPDATE barista_points SET point_id = $2 WHERE user_id = $1`,
			baristaID, *patch.PointID,
		); err != nil {
			return b, fmt.Errorf("admin: barista rebind: %w", err)
		}
	}
	if patch.PasswordHash != nil {
		if _, err := tx.Exec(ctx,
			`DELETE FROM refresh_tokens WHERE user_id = $1`, baristaID,
		); err != nil {
			return b, fmt.Errorf("admin: revoke barista sessions: %w", err)
		}
	}

	err = tx.QueryRow(ctx, `
		SELECT u.id, u.email, bp.point_id, p.title, bp.created_at::text, u.password_enc
		FROM barista_points bp
		JOIN users u ON u.id = bp.user_id
		JOIN points p ON p.id = bp.point_id
		WHERE bp.user_id = $1`, baristaID,
	).Scan(&b.ID, &b.Email, &b.PointID, &b.PointTitle, &b.CreatedAt, &b.PasswordEnc)
	if err != nil {
		return b, fmt.Errorf("admin: barista reload: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return b, fmt.Errorf("admin: barista update commit: %w", err)
	}
	return b, nil
}

// ListMenu — все позиции меню точки, включая скрытые из продажи.
// ListMenu — меню точки (включая скрытые позиции) вместе с добавками товаров.
func (s *Store) ListMenu(ctx context.Context, ownerID, pointID string) ([]MenuItem, error) {
	owned, err := s.PointOwnedBy(ctx, ownerID, pointID)
	if err != nil {
		return nil, fmt.Errorf("admin: menu owner check: %w", err)
	}
	if !owned {
		return nil, ErrPointNotFound
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, title, price_minor, prep_seconds, is_available
		FROM menu_items WHERE point_id = $1
		ORDER BY title`, pointID)
	if err != nil {
		return nil, fmt.Errorf("admin: menu list: %w", err)
	}
	defer rows.Close()
	items := []MenuItem{}
	ids := make([]string, 0)
	for rows.Next() {
		var m MenuItem
		if err := rows.Scan(&m.ID, &m.Title, &m.PriceMinor, &m.PrepSeconds, &m.IsAvailable); err != nil {
			return nil, fmt.Errorf("admin: menu scan: %w", err)
		}
		m.Addons = []Addon{}
		items = append(items, m)
		ids = append(ids, m.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return items, nil
	}

	addonRows, err := s.pool.Query(ctx, `
		SELECT id, menu_item_id, title, price_minor, prep_seconds, is_available
		FROM addons WHERE menu_item_id = ANY($1::uuid[])
		ORDER BY title`, ids)
	if err != nil {
		return nil, fmt.Errorf("admin: addons list: %w", err)
	}
	defer addonRows.Close()
	index := make(map[string]int, len(items))
	for i, m := range items {
		index[m.ID] = i
	}
	for addonRows.Next() {
		var a Addon
		if err := addonRows.Scan(&a.ID, &a.MenuItemID, &a.Title, &a.PriceMinor,
			&a.PrepSeconds, &a.IsAvailable); err != nil {
			return nil, fmt.Errorf("admin: addons scan: %w", err)
		}
		if i, ok := index[a.MenuItemID]; ok {
			items[i].Addons = append(items[i].Addons, a)
		}
	}
	return items, addonRows.Err()
}

// CreateMenuItem — новая позиция меню точки.
func (s *Store) CreateMenuItem(ctx context.Context, ownerID, pointID, title string, priceMinor int64, prepSeconds int) (MenuItem, error) {
	var m MenuItem
	owned, err := s.PointOwnedBy(ctx, ownerID, pointID)
	if err != nil {
		return m, fmt.Errorf("admin: menu owner check: %w", err)
	}
	if !owned {
		return m, ErrPointNotFound
	}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO menu_items (point_id, title, price_minor, prep_seconds)
		VALUES ($1, $2, $3, $4)
		RETURNING id, title, price_minor, prep_seconds, is_available`,
		pointID, title, priceMinor, prepSeconds,
	).Scan(&m.ID, &m.Title, &m.PriceMinor, &m.PrepSeconds, &m.IsAvailable)
	if err != nil {
		return m, fmt.Errorf("admin: menu insert: %w", err)
	}
	return m, nil
}

// MenuItemPatch — частичное изменение позиции меню.
type MenuItemPatch struct {
	Title       *string
	PriceMinor  *int64
	PrepSeconds *int
	IsAvailable *bool
}

// UpdateMenuItem — меняет название, цену, время приготовления или видимость.
func (s *Store) UpdateMenuItem(ctx context.Context, ownerID, itemID string, patch MenuItemPatch) (MenuItem, error) {
	var m MenuItem
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return m, fmt.Errorf("admin: menu update tx: %w", err)
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `
		SELECT mi.id, mi.title, mi.price_minor, mi.prep_seconds, mi.is_available
		FROM menu_items mi
		JOIN points p ON p.id = mi.point_id
		WHERE mi.id = $1 AND p.owner_id = $2
		FOR UPDATE OF mi`, itemID, ownerID,
	).Scan(&m.ID, &m.Title, &m.PriceMinor, &m.PrepSeconds, &m.IsAvailable)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrMenuItemMissing
	}
	if err != nil {
		return m, fmt.Errorf("admin: menu update load: %w", err)
	}

	if patch.Title != nil {
		m.Title = *patch.Title
	}
	if patch.PriceMinor != nil {
		m.PriceMinor = *patch.PriceMinor
	}
	if patch.PrepSeconds != nil {
		m.PrepSeconds = *patch.PrepSeconds
	}
	if patch.IsAvailable != nil {
		m.IsAvailable = *patch.IsAvailable
	}

	if _, err := tx.Exec(ctx, `
		UPDATE menu_items
		SET title = $2, price_minor = $3, prep_seconds = $4, is_available = $5
		WHERE id = $1`,
		m.ID, m.Title, m.PriceMinor, m.PrepSeconds, m.IsAvailable,
	); err != nil {
		return m, fmt.Errorf("admin: menu update: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return m, fmt.Errorf("admin: menu update commit: %w", err)
	}
	return m, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// CreateAddon — новая добавка к товару. Товар обязан принадлежать точке
// владельца: чужие меню недоступны.
func (s *Store) CreateAddon(ctx context.Context, ownerID, menuItemID, title string, priceMinor int64, prepSeconds int) (Addon, error) {
	var a Addon
	var pointID string
	err := s.pool.QueryRow(ctx, `
		SELECT mi.point_id
		FROM menu_items mi JOIN points p ON p.id = mi.point_id
		WHERE mi.id = $1 AND p.owner_id = $2`, menuItemID, ownerID,
	).Scan(&pointID)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrMenuItemMissing
	}
	if err != nil {
		return a, fmt.Errorf("admin: addon menu item check: %w", err)
	}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO addons (menu_item_id, title, price_minor, prep_seconds)
		VALUES ($1, $2, $3, $4)
		RETURNING id, menu_item_id, title, price_minor, prep_seconds, is_available`,
		menuItemID, title, priceMinor, prepSeconds,
	).Scan(&a.ID, &a.MenuItemID, &a.Title, &a.PriceMinor, &a.PrepSeconds, &a.IsAvailable)
	if err != nil {
		return a, fmt.Errorf("admin: addon insert: %w", err)
	}
	return a, nil
}

// AddonPatch — частичное изменение добавки.
type AddonPatch struct {
	Title       *string
	PriceMinor  *int64
	PrepSeconds *int
	IsAvailable *bool
}

// UpdateAddon — меняет название, цену, время приготовления или видимость.
func (s *Store) UpdateAddon(ctx context.Context, ownerID, addonID string, patch AddonPatch) (Addon, error) {
	var a Addon
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return a, fmt.Errorf("admin: addon update tx: %w", err)
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `
		SELECT a.id, a.menu_item_id, a.title, a.price_minor, a.prep_seconds, a.is_available
		FROM addons a
		JOIN menu_items mi ON mi.id = a.menu_item_id
		JOIN points p ON p.id = mi.point_id
		WHERE a.id = $1 AND p.owner_id = $2
		FOR UPDATE OF a`, addonID, ownerID,
	).Scan(&a.ID, &a.MenuItemID, &a.Title, &a.PriceMinor, &a.PrepSeconds, &a.IsAvailable)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrAddonMissing
	}
	if err != nil {
		return a, fmt.Errorf("admin: addon update load: %w", err)
	}

	if patch.Title != nil {
		a.Title = *patch.Title
	}
	if patch.PriceMinor != nil {
		a.PriceMinor = *patch.PriceMinor
	}
	if patch.PrepSeconds != nil {
		a.PrepSeconds = *patch.PrepSeconds
	}
	if patch.IsAvailable != nil {
		a.IsAvailable = *patch.IsAvailable
	}

	if _, err := tx.Exec(ctx, `
		UPDATE addons
		SET title = $2, price_minor = $3, prep_seconds = $4, is_available = $5
		WHERE id = $1`,
		a.ID, a.Title, a.PriceMinor, a.PrepSeconds, a.IsAvailable,
	); err != nil {
		return a, fmt.Errorf("admin: addon update: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return a, fmt.Errorf("admin: addon update commit: %w", err)
	}
	return a, nil
}

// DeleteAddon — удаляет добавку, которая ещё не попала в историю заказов
// (в order_item_addons хранится снимок). Чужую добавку вернёт addon_not_found.
func (s *Store) DeleteAddon(ctx context.Context, ownerID, addonID string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM addons a
		USING menu_items mi, points p
		WHERE a.id = $1 AND mi.id = a.menu_item_id AND p.id = mi.point_id AND p.owner_id = $2`,
		addonID, ownerID)
	if err != nil {
		return fmt.Errorf("admin: addon delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAddonMissing
	}
	return nil
}

// Profile — настройки владельца (название кофейни).
func (s *Store) Profile(ctx context.Context, ownerID string) (Profile, error) {
	var p Profile
	err := s.pool.QueryRow(ctx,
		`SELECT brand_name FROM users WHERE id = $1 AND role = 'owner'`, ownerID,
	).Scan(&p.BrandName)
	if err != nil {
		return p, fmt.Errorf("admin: profile: %w", err)
	}
	return p, nil
}

// UpdateProfile — сохраняет название кофейни.
func (s *Store) UpdateProfile(ctx context.Context, ownerID, brandName string) (Profile, error) {
	var p Profile
	err := s.pool.QueryRow(ctx, `
		UPDATE users SET brand_name = $2
		WHERE id = $1 AND role = 'owner'
		RETURNING brand_name`, ownerID, brandName,
	).Scan(&p.BrandName)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, httpx.ErrForbidden
	}
	if err != nil {
		return p, fmt.Errorf("admin: profile update: %w", err)
	}
	return p, nil
}
