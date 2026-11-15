// Package points — точки, меню и движок ёмкости v1 (веха M4).
// Движок (ядро AIK-11): лимит заказов на N-минутное окно из capacity_config.
package points

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
	ErrPointNotFound = httpx.NewError(404, "point_not_found", "точка не найдена")
	ErrWindowFull    = httpx.NewError(409, "window_full", "окно выдачи заполнено")
)

// CapacityConfig — параметры ёмкости точки (jsonb capacity_config).
type CapacityConfig struct {
	WindowMinutes      int    // длина окна выдачи
	MaxOrdersPerWindow int    // лимит заказов на окно
	WorkStart          string // "HH:MM" локального времени точки
	WorkEnd            string
}

// defaults — соло-режим AIK-13: разумные значения без настройки.
func defaultCapacity() CapacityConfig {
	return CapacityConfig{WindowMinutes: 5, MaxOrdersPerWindow: 3, WorkStart: "08:00", WorkEnd: "20:00"}
}

func parseCapacity(raw []byte) CapacityConfig {
	cfg := defaultCapacity()
	if len(raw) == 0 {
		return cfg
	}
	var stored struct {
		WindowMinutes      *int    `json:"window_minutes"`
		MaxOrdersPerWindow *int    `json:"max_orders_per_window"`
		WorkStart          *string `json:"work_start"`
		WorkEnd            *string `json:"work_end"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		return cfg // битый конфиг = дефолт, точка продолжает работать
	}
	if stored.WindowMinutes != nil && *stored.WindowMinutes > 0 {
		cfg.WindowMinutes = *stored.WindowMinutes
	}
	if stored.MaxOrdersPerWindow != nil && *stored.MaxOrdersPerWindow > 0 {
		cfg.MaxOrdersPerWindow = *stored.MaxOrdersPerWindow
	}
	if stored.WorkStart != nil {
		cfg.WorkStart = *stored.WorkStart
	}
	if stored.WorkEnd != nil {
		cfg.WorkEnd = *stored.WorkEnd
	}
	return cfg
}

// Point — точка выдачи для клиента.
type Point struct {
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Address  string          `json:"address"`
	Geo      json.RawMessage `json:"geo"`
	Timezone string          `json:"timezone"`
}

// MenuItem — позиция меню точки.
type MenuItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	PriceMinor  int64  `json:"price_minor"`
	PrepSeconds int    `json:"prep_seconds"`
}

// Window — окно выдачи со свободной ёмкостью.
type Window struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	Free  int       `json:"free"`
	Passed bool     `json:"passed"`
}

// Store — SQL для точек, меню и ёмкости.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// List — активные точки.
func (s *Store) List(ctx context.Context) ([]Point, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, title, COALESCE(address, ''), geo, timezone
		FROM points WHERE is_active ORDER BY title`)
	if err != nil {
		return nil, fmt.Errorf("points: list: %w", err)
	}
	defer rows.Close()
	var out []Point
	for rows.Next() {
		var p Point
		var geo []byte
		if err := rows.Scan(&p.ID, &p.Title, &p.Address, &geo, &p.Timezone); err != nil {
			return nil, fmt.Errorf("points: scan: %w", err)
		}
		p.Geo = json.RawMessage(geo)
		out = append(out, p)
	}
	return out, rows.Err()
}

// Menu — доступные позиции точки.
func (s *Store) Menu(ctx context.Context, pointID string) ([]MenuItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, title, price_minor, prep_seconds
		FROM menu_items WHERE point_id = $1 AND is_available ORDER BY title`, pointID)
	if err != nil {
		return nil, fmt.Errorf("points: menu: %w", err)
	}
	defer rows.Close()
	var out []MenuItem
	for rows.Next() {
		var m MenuItem
		if err := rows.Scan(&m.ID, &m.Title, &m.PriceMinor, &m.PrepSeconds); err != nil {
			return nil, fmt.Errorf("points: scan menu: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// pointTZ — конфиг и таймзона точки.
func (s *Store) pointConfig(ctx context.Context, pointID string) (CapacityConfig, *time.Location, error) {
	var raw []byte
	var tz string
	err := s.pool.QueryRow(ctx,
		`SELECT capacity_config, timezone FROM points WHERE id = $1 AND is_active`, pointID,
	).Scan(&raw, &tz)
	if err == pgx.ErrNoRows {
		return CapacityConfig{}, nil, ErrPointNotFound
	}
	if err != nil {
		return CapacityConfig{}, nil, fmt.Errorf("points: config: %w", err)
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return CapacityConfig{}, nil, fmt.Errorf("points: timezone %q: %w", tz, err)
	}
	return parseCapacity(raw), loc, nil
}

// Windows — окна выдачи точки на день (в таймзоне точки) со свободной ёмкостью.
// day — любая минута искомого дня (обычно now).
func (s *Store) Windows(ctx context.Context, pointID string, day time.Time) ([]Window, error) {
	capacity, loc, err := s.pointConfig(ctx, pointID)
	if err != nil {
		return nil, err
	}

	startMin, err := parseHM(capacity.WorkStart)
	if err != nil {
		return nil, fmt.Errorf("points: work_start: %w", err)
	}
	endMin, err := parseHM(capacity.WorkEnd)
	if err != nil {
		return nil, fmt.Errorf("points: work_end: %w", err)
	}

	dayInTZ := day.In(loc)
	dayStart := time.Date(dayInTZ.Year(), dayInTZ.Month(), dayInTZ.Day(), 0, 0, 0, 0, loc)
	windowDur := time.Duration(capacity.WindowMinutes) * time.Minute

	var out []Window
	for offset := startMin; offset < endMin; offset += capacity.WindowMinutes {
		wStart := dayStart.Add(time.Duration(offset) * time.Minute)
		wEnd := wStart.Add(windowDur)

		var taken int
		if err := s.pool.QueryRow(ctx, `
			SELECT count(*) FROM orders
			WHERE point_id = $1 AND status <> 'cancelled'
			  AND pickup_slot_start >= $2 AND pickup_slot_start < $3`,
			pointID, wStart, wEnd,
		).Scan(&taken); err != nil {
			return nil, fmt.Errorf("points: window count: %w", err)
		}
		free := capacity.MaxOrdersPerWindow - taken
		if free < 0 {
			free = 0
		}
		out = append(out, Window{
			Start:  wStart,
			End:    wEnd,
			Free:   free,
			Passed: time.Now().After(wEnd),
		})
	}
	return out, nil
}

// CheckWindowCapacity — проверка ёмкости окна внутри транзакции создания заказа.
// pg_advisory_xact_lock закрывает гонку двух клиентов на последнее место.
func (s *Store) CheckWindowCapacity(ctx context.Context, tx pgx.Tx, pointID string, slotStart time.Time) error {
	capacity, loc, err := s.pointConfig(ctx, pointID)
	if err != nil {
		return err
	}
	slotInTZ := slotStart.In(loc)
	windowStart := time.Date(slotInTZ.Year(), slotInTZ.Month(), slotInTZ.Day(), slotInTZ.Hour(), slotInTZ.Minute(), 0, 0, loc).
		Truncate(time.Duration(capacity.WindowMinutes) * time.Minute)
	// Truncate работает по UTC; пересчитываем окно от локального времени корректно:
	minutesOfDay := slotInTZ.Hour()*60 + slotInTZ.Minute()
	windowStart = time.Date(slotInTZ.Year(), slotInTZ.Month(), slotInTZ.Day(), 0, 0, 0, 0, loc).
		Add(time.Duration((minutesOfDay/capacity.WindowMinutes)*capacity.WindowMinutes) * time.Minute)
	windowEnd := windowStart.Add(time.Duration(capacity.WindowMinutes) * time.Minute)

	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		pointID+"|"+windowStart.Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("points: advisory lock: %w", err)
	}

	var taken int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM orders
		WHERE point_id = $1 AND status <> 'cancelled'
		  AND pickup_slot_start >= $2 AND pickup_slot_start < $3`,
		pointID, windowStart, windowEnd,
	).Scan(&taken); err != nil {
		return fmt.Errorf("points: capacity count: %w", err)
	}
	if taken >= capacity.MaxOrdersPerWindow {
		return ErrWindowFull
	}
	return nil
}

func parseHM(hm string) (int, error) {
	var h, m int
	if _, err := fmt.Sscanf(hm, "%d:%d", &h, &m); err != nil {
		return 0, fmt.Errorf("формат HH:MM: %w", err)
	}
	return h*60 + m, nil
}
