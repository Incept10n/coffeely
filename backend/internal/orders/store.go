// Package orders — заказы, KDS-очередь баристы, метрики дедлайнов (веха M3).
// Движок ёмкости (лимит заказов на окно) подключается в M4.
package orders

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Incept10n/coffeely/backend/internal/httpx"
)

var (
	ErrOrderNotFound    = httpx.NewError(404, "order_not_found", "заказ не найден")
	ErrPointNotFound    = httpx.NewError(404, "point_not_found", "точка не найдена")
	ErrForbiddenOrder   = httpx.NewError(403, "forbidden_order", "нет доступа к этому заказу")
	ErrBadTransition    = httpx.NewError(409, "bad_transition", "недопустимый переход статуса")
	ErrEmptyOrder       = httpx.NewError(400, "empty_order", "заказ без позиций")
	ErrPointInactive    = httpx.NewError(409, "point_inactive", "точка неактивна")
	ErrBadPickupWindow  = httpx.NewError(400, "bad_pickup_window", "некорректное окно выдачи")
	ErrMenuItemNotFound = httpx.NewError(400, "menu_item_not_found", "позиция меню недоступна в этой точке")
)

// Статусы заказов (совпадают с enum в БД).
const (
	StatusCreated       = "created"
	StatusPaid          = "paid"
	StatusInProduction  = "in_production"
	StatusReady         = "ready"
	StatusGivenOut      = "given_out"
	StatusCancelled     = "cancelled"
)

// allowedTransitions — граф переходов, ведут бариста (M3) и клиент (отмена в M4).
var allowedTransitions = map[string]map[string]bool{
	StatusPaid:         {StatusInProduction: true, StatusCancelled: true},
	StatusInProduction: {StatusReady: true},
	StatusReady:        {StatusGivenOut: true},
}

// Order — заказ как он уходит в API.
type Order struct {
	ID          string    `json:"id"`
	Number      int64     `json:"number"`
	PointID     string    `json:"point_id"`
	ClientID    string    `json:"client_id"`
	Status      string    `json:"status"`
	SlotStart   time.Time `json:"pickup_slot_start"`
	SlotEnd     time.Time `json:"pickup_slot_end"`
	Deadline    time.Time `json:"deadline_at"`
	TotalMinor  int64     `json:"total_minor"`
	CreatedAt   time.Time `json:"created_at"`
	Items       []Item    `json:"items,omitempty"`
}

// Item — позиция заказа.
type Item struct {
	MenuItemID string `json:"menu_item_id"`
	Title      string `json:"title"`
	Qty        int    `json:"qty"`
	PriceMinor int64  `json:"price_minor"`
}

// Store — SQL для заказов.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Create создаёт заказ (v1: статус paid — «оплата в кассе при получении», M4
// добавит проверку ёмкости окна). deadline_at = начало окна − сумма prep_seconds.
func (s *Store) Create(ctx context.Context, pointID, clientID string, slotStart, slotEnd time.Time, items []Item) (Order, error) {
	if len(items) == 0 {
		return Order{}, ErrEmptyOrder
	}
	if !slotEnd.After(slotStart) {
		return Order{}, ErrBadPickupWindow
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("orders: create tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Блокируем точку и меню в одной транзакции.
	var pointActive bool
	if err := tx.QueryRow(ctx,
		`SELECT is_active FROM points WHERE id = $1 FOR UPDATE`, pointID,
	).Scan(&pointActive); err != nil {
		if err == pgx.ErrNoRows {
			return Order{}, ErrPointNotFound
		}
		return Order{}, fmt.Errorf("orders: point: %w", err)
	}
	if !pointActive {
		return Order{}, ErrPointInactive
	}

	var deadline time.Time
	var total int64
	for _, it := range items {
		var prep int
		var price int64
		err := tx.QueryRow(ctx, `
			SELECT prep_seconds, price_minor FROM menu_items
			WHERE id = $1 AND point_id = $2 AND is_available`, it.MenuItemID, pointID,
		).Scan(&prep, &price)
		if err != nil {
			if err == pgx.ErrNoRows {
				return Order{}, fmt.Errorf("%w: %s", ErrMenuItemNotFound, it.MenuItemID)
			}
			return Order{}, fmt.Errorf("orders: menu item: %w", err)
		}
		if it.Qty <= 0 {
			return Order{}, ErrEmptyOrder
		}
		total += price * int64(it.Qty)
		if prep*it.Qty > 0 {
			d := slotStart.Add(-time.Duration(prep*it.Qty) * time.Second)
			if deadline.IsZero() || d.Before(deadline) {
				deadline = d
			}
		}
	}
	if deadline.IsZero() {
		deadline = slotStart
	}

	o := Order{
		PointID: pointID, ClientID: clientID, Status: StatusPaid,
		SlotStart: slotStart, SlotEnd: slotEnd, Deadline: deadline, TotalMinor: total,
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO orders (point_id, client_id, status, pickup_slot_start, pickup_slot_end, deadline_at, total_minor)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, order_number, created_at`,
		pointID, clientID, StatusPaid, slotStart, slotEnd, deadline, total,
	).Scan(&o.ID, &o.Number, &o.CreatedAt)
	if err != nil {
		return Order{}, fmt.Errorf("orders: insert: %w", err)
	}

	for _, it := range items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO order_items (order_id, menu_item_id, qty, price_minor)
			SELECT $1, mi.id, $2, mi.price_minor FROM menu_items mi WHERE mi.id = $3`,
			o.ID, it.Qty, it.MenuItemID,
		); err != nil {
			return Order{}, fmt.Errorf("orders: insert item: %w", err)
		}
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO order_events (order_id, event, actor_id) VALUES ($1, $2, $3)`,
		o.ID, StatusPaid, clientID,
	); err != nil {
		return Order{}, fmt.Errorf("orders: event paid: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("orders: commit: %w", err)
	}
	return o, nil
}

// ByID возвращает заказ с позициями; для клиента — только свой.
func (s *Store) ByID(ctx context.Context, id string) (Order, error) {
	o, err := s.scanOrder(s.pool.QueryRow(ctx, `
		SELECT id, order_number, point_id, client_id, status,
		       pickup_slot_start, pickup_slot_end, deadline_at, total_minor, created_at
		FROM orders WHERE id = $1`, id))
	if err != nil {
		return Order{}, err
	}
	o.Items, err = s.items(ctx, o.ID)
	if err != nil {
		return Order{}, err
	}
	return o, nil
}

// QueueOfPoint — активные заказы точки по дедлайну (для KDS).
func (s *Store) QueueOfPoint(ctx context.Context, pointID string) ([]Order, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, order_number, point_id, client_id, status,
		       pickup_slot_start, pickup_slot_end, deadline_at, total_minor, created_at
		FROM orders
		WHERE point_id = $1 AND status IN ($2, $3, $4)
		ORDER BY deadline_at`,
		pointID, StatusPaid, StatusInProduction, StatusReady,
	)
	if err != nil {
		return nil, fmt.Errorf("orders: queue: %w", err)
	}
	defer rows.Close()

	var out []Order
	for rows.Next() {
		o, err := s.scanOrder(rows)
		if err != nil {
			return nil, err
		}
		if o.Items, err = s.items(ctx, o.ID); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// PointOfBarista — точка баристы (один бариста — одна точка).
func (s *Store) PointOfBarista(ctx context.Context, userID string) (string, error) {
	var pointID string
	err := s.pool.QueryRow(ctx,
		`SELECT point_id FROM barista_points WHERE user_id = $1`, userID,
	).Scan(&pointID)
	if err == pgx.ErrNoRows {
		return "", httpx.NewError(404, "no_point", "за баристой не закреплена точка")
	}
	if err != nil {
		return "", fmt.Errorf("orders: point of barista: %w", err)
	}
	return pointID, nil
}

// Transition меняет статус с валидацией перехода и пишет событие; при промахе
// дедлайна добавляет событие deadline_missed (метрики AIK-14 §6).
func (s *Store) Transition(ctx context.Context, orderID, actorID, next string, now time.Time) (Order, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("orders: transition tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var current, pointID string
	err = tx.QueryRow(ctx,
		`SELECT status, point_id FROM orders WHERE id = $1 FOR UPDATE`, orderID,
	).Scan(&current, &pointID)
	if err == pgx.ErrNoRows {
		return Order{}, ErrOrderNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("orders: load: %w", err)
	}
	if !allowedTransitions[current][next] {
		return Order{}, fmt.Errorf("%s -> %s: %w", current, next, ErrBadTransition)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE orders SET status = $2 WHERE id = $1`, orderID, next,
	); err != nil {
		return Order{}, fmt.Errorf("orders: update status: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO order_events (order_id, event, at, actor_id) VALUES ($1, $2, $3, $4)`,
		orderID, next, now, actorID,
	); err != nil {
		return Order{}, fmt.Errorf("orders: insert event: %w", err)
	}

	if next == StatusGivenOut {
		var deadline time.Time
		if err := tx.QueryRow(ctx,
			`SELECT deadline_at FROM orders WHERE id = $1`, orderID,
		).Scan(&deadline); err != nil {
			return Order{}, fmt.Errorf("orders: deadline: %w", err)
		}
		if now.After(deadline) {
			if _, err := tx.Exec(ctx,
				`INSERT INTO order_events (order_id, event, at, actor_id) VALUES ($1, 'deadline_missed', $2, $3)`,
				orderID, now, actorID,
			); err != nil {
				return Order{}, fmt.Errorf("orders: insert missed: %w", err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("orders: commit: %w", err)
	}
	return s.ByID(ctx, orderID)
}

// Metrics — соблюдённые/промаханные дедлайны точки за день (в таймзоне точки).
func (s *Store) Metrics(ctx context.Context, pointID string, day time.Time) (onTime, missed int, err error) {
	err = s.pool.QueryRow(ctx, `
		WITH outs AS (
			SELECT DISTINCT ON (ev.order_id)
			       ev.order_id, (ev.at <= o.deadline_at) AS on_time
			FROM order_events ev
			JOIN orders o ON o.id = ev.order_id
			JOIN points p ON p.id = o.point_id
			WHERE o.point_id = $1 AND ev.event = 'given_out'
			  AND (ev.at AT TIME ZONE p.timezone)::date = ($2::timestamptz AT TIME ZONE p.timezone)::date
		)
		SELECT count(*) FILTER (WHERE on_time), count(*) FILTER (WHERE NOT on_time) FROM outs`,
		pointID, day,
	).Scan(&onTime, &missed)
	if err != nil {
		return 0, 0, fmt.Errorf("orders: metrics: %w", err)
	}
	return onTime, missed, nil
}

func (s *Store) scanOrder(row pgx.Row) (Order, error) {
	var o Order
	err := row.Scan(&o.ID, &o.Number, &o.PointID, &o.ClientID, &o.Status,
		&o.SlotStart, &o.SlotEnd, &o.Deadline, &o.TotalMinor, &o.CreatedAt)
	if err == pgx.ErrNoRows {
		return Order{}, ErrOrderNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("orders: scan: %w", err)
	}
	return o, nil
}

func (s *Store) items(ctx context.Context, orderID string) ([]Item, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT oi.menu_item_id, mi.title, oi.qty, oi.price_minor
		FROM order_items oi JOIN menu_items mi ON mi.id = oi.menu_item_id
		WHERE oi.order_id = $1`, orderID)
	if err != nil {
		return nil, fmt.Errorf("orders: items: %w", err)
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.MenuItemID, &it.Title, &it.Qty, &it.PriceMinor); err != nil {
			return nil, fmt.Errorf("orders: scan item: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// jsonRaw — marshaled event payload для публикации в WS.
func jsonRaw(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return raw
}
