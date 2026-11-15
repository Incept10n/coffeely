// Package points — точки, меню и движок ёмкости (ядро AIK-11).
//
// v1 клиент выбирал окно выдачи сам. v2 отказался от этого: клиент нажимает
// кофейню, видит примерное время ожидания, а точное окно производства точка
// назначает сама — по текущей очереди и ёмкости (PlanSlot).
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
	ErrPointBusy     = httpx.NewError(409, "point_busy", "точка перегружена, попробуйте позже")
)

// maxLead — дальше этого от «сейчас» заказ не назначаем: лучше честно отказать,
// чем пообещать кофе через три часа.
const maxLead = 120 * time.Minute

// CapacityConfig — параметры ёмкости точки (jsonb capacity_config).
type CapacityConfig struct {
	WindowMinutes      int    // длина окна производства
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

// Load — текущая загрузка точки: сколько ждать, если заказать прямо сейчас.
type Load struct {
	// WaitSeconds — работа, которая уже стоит в очереди (сумма prep активных заказов).
	WaitSeconds int `json:"wait_seconds"`
	// ActiveOrders — заказов в работе (paid + in_production).
	ActiveOrders int `json:"active_orders"`
	// IsOpen — точка сейчас принимает заказы (в рабочих часах).
	IsOpen bool `json:"is_open"`
	// OpensAt — "HH:MM", когда откроется (только если закрыто).
	OpensAt string `json:"opens_at,omitempty"`
}

// Point — точка выдачи для клиента.
type Point struct {
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Address  string          `json:"address"`
	Geo      json.RawMessage `json:"geo"`
	Timezone string          `json:"timezone"`
	Load     Load            `json:"load"`
}

// MenuItem — позиция меню точки.
type MenuItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	PriceMinor  int64  `json:"price_minor"`
	PrepSeconds int    `json:"prep_seconds"`
}

// SlotPlan — назначенное точкой окно производства.
type SlotPlan struct {
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	Deadline     time.Time `json:"deadline"`
	QueueSeconds int       `json:"queue_seconds"`
}

// Store — SQL для точек, меню и ёмкости.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// List — активные точки с текущей загрузкой (ожидание, число заказов, открыто ли).
func (s *Store) List(ctx context.Context, now time.Time) ([]Point, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.title, COALESCE(p.address, ''), p.geo, p.timezone, p.capacity_config,
		       COALESCE(q.prep_seconds, 0), COALESCE(q.active_orders, 0)
		FROM points p
		LEFT JOIN LATERAL (
			SELECT COALESCE(SUM(mi.prep_seconds * oi.qty), 0)::int AS prep_seconds,
			       COUNT(DISTINCT o.id)::int AS active_orders
			FROM orders o
			JOIN order_items oi ON oi.order_id = o.id
			JOIN menu_items mi ON mi.id = oi.menu_item_id
			WHERE o.point_id = p.id AND o.status IN ('paid', 'in_production')
		) q ON true
		WHERE p.is_active
		ORDER BY p.title`)
	if err != nil {
		return nil, fmt.Errorf("points: list: %w", err)
	}
	defer rows.Close()

	var out []Point
	for rows.Next() {
		var p Point
		var geo, capacityRaw []byte
		if err := rows.Scan(&p.ID, &p.Title, &p.Address, &geo, &p.Timezone, &capacityRaw,
			&p.Load.WaitSeconds, &p.Load.ActiveOrders); err != nil {
			return nil, fmt.Errorf("points: scan: %w", err)
		}
		p.Geo = json.RawMessage(geo)
		p.Load.IsOpen, p.Load.OpensAt = openState(parseCapacity(capacityRaw), p.Timezone, now)
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

// PlanSlot — движок ёмкости v2: назначает ближайшее окно, в котором точка
// успеет сделать заказ, с учётом того, что уже стоит в очереди.
//
// Модель: точка — один производственный поток. Время готовности =
// «сейчас» + работа в очереди + собственный prep заказа, округлённое вверх до
// сетки окон точки. Если выбранное окно заполнено (лимит заказов), берётся
// следующее. Дальше maxLead — отказ point_busy.
//
// Вызывается внутри транзакции создания заказа; advisory-lock по точке
// закрывает гонку двух клиентов на одно окно.
func (s *Store) PlanSlot(ctx context.Context, tx pgx.Tx, pointID string, ownPrep time.Duration, now time.Time) (SlotPlan, error) {
	capacity, loc, err := pointConfigTx(ctx, tx, pointID)
	if err != nil {
		return SlotPlan{}, err
	}

	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, pointID+"|plan",
	); err != nil {
		return SlotPlan{}, fmt.Errorf("points: advisory lock: %w", err)
	}

	var queueSeconds int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(mi.prep_seconds * oi.qty), 0)::int
		FROM orders o
		JOIN order_items oi ON oi.order_id = o.id
		JOIN menu_items mi ON mi.id = oi.menu_item_id
		WHERE o.point_id = $1 AND o.status IN ('paid', 'in_production')`, pointID,
	).Scan(&queueSeconds); err != nil {
		return SlotPlan{}, fmt.Errorf("points: queue prep: %w", err)
	}

	startMin, err := parseHM(capacity.WorkStart)
	if err != nil {
		return SlotPlan{}, fmt.Errorf("points: work_start: %w", err)
	}
	endMin, err := parseHM(capacity.WorkEnd)
	if err != nil {
		return SlotPlan{}, fmt.Errorf("points: work_end: %w", err)
	}

	ready := now.Add(time.Duration(queueSeconds)*time.Second + ownPrep)
	if queueSeconds == 0 && ownPrep == 0 {
		ready = now
	}

	// Не больше окна на день вперёд: 24 ч / 5 мин = 288 окон, запас — 512 итераций.
	deadlineLimit := now.Add(maxLead)
	candidate := ready
	for i := 0; i < 512; i++ {
		if candidate.After(deadlineLimit) {
			break
		}
		wStart, wEnd, open := windowAt(candidate, loc, startMin, endMin, capacity.WindowMinutes)
		if !open {
			candidate = nextOpening(candidate, loc, startMin)
			continue
		}
		// Окно должно начинаться не раньше, чем точка освободится: бариста
		// получает полный prep заказа ДО начала окна (ядро AIK-11).
		if wStart.Before(ready) {
			candidate = wEnd
			continue
		}

		var taken int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM orders
			WHERE point_id = $1 AND status <> 'cancelled'
			  AND pickup_slot_start >= $2 AND pickup_slot_start < $3`,
			pointID, wStart, wEnd,
		).Scan(&taken); err != nil {
			return SlotPlan{}, fmt.Errorf("points: window count: %w", err)
		}
		if taken < capacity.MaxOrdersPerWindow {
			return SlotPlan{
				Start:        wStart,
				End:          wEnd,
				Deadline:     wStart.Add(-ownPrep),
				QueueSeconds: queueSeconds,
			}, nil
		}
		candidate = wEnd
	}
	return SlotPlan{}, ErrPointBusy
}

// pointConfigTx — конфиг и таймзона точки внутри транзакции (создание заказа).
func pointConfigTx(ctx context.Context, tx pgx.Tx, pointID string) (CapacityConfig, *time.Location, error) {
	var raw []byte
	var tz string
	err := tx.QueryRow(ctx,
		`SELECT capacity_config, timezone FROM points WHERE id = $1 AND is_active`, pointID,
	).Scan(&raw, &tz)
	if err == pgx.ErrNoRows {
		return CapacityConfig{}, nil, ErrPointNotFound
	}
	if err != nil {
		return CapacityConfig{}, nil, fmt.Errorf("points: config: %w", err)
	}
	return capacityWithLocation(raw, tz)
}

func capacityWithLocation(raw []byte, tz string) (CapacityConfig, *time.Location, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return CapacityConfig{}, nil, fmt.Errorf("points: timezone %q: %w", tz, err)
	}
	return parseCapacity(raw), loc, nil
}

// openState — открыта ли точка в момент now (в её таймзоне).
func openState(capacity CapacityConfig, tz string, now time.Time) (bool, string) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return false, capacity.WorkStart
	}
	startMin, err1 := parseHM(capacity.WorkStart)
	endMin, err2 := parseHM(capacity.WorkEnd)
	if err1 != nil || err2 != nil {
		return false, ""
	}
	local := now.In(loc)
	minutes := local.Hour()*60 + local.Minute()
	if minutes < startMin || minutes >= endMin {
		return false, capacity.WorkStart
	}
	return true, ""
}

// windowAt — окно сетки, в которое попадает момент ts (или первое окно дня).
// open=false — ts вне рабочих часов.
func windowAt(ts time.Time, loc *time.Location, startMin, endMin, windowMinutes int) (start, end time.Time, open bool) {
	if windowMinutes <= 0 {
		windowMinutes = 5
	}
	local := ts.In(loc)
	minutes := local.Hour()*60 + local.Minute()
	if minutes >= endMin {
		return time.Time{}, time.Time{}, false
	}
	dayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	offset := startMin
	if minutes > startMin {
		offset = startMin + ((minutes-startMin)/windowMinutes)*windowMinutes
	}
	wStart := dayStart.Add(time.Duration(offset) * time.Minute)
	return wStart, wStart.Add(time.Duration(windowMinutes) * time.Minute), true
}

// nextOpening — начало рабочего дня, следующего за ts.
func nextOpening(ts time.Time, loc *time.Location, startMin int) time.Time {
	local := ts.In(loc)
	minutes := local.Hour()*60 + local.Minute()
	dayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	if minutes < startMin {
		return dayStart.Add(time.Duration(startMin) * time.Minute)
	}
	return dayStart.AddDate(0, 0, 1).Add(time.Duration(startMin) * time.Minute)
}

func parseHM(hm string) (int, error) {
	var h, m int
	if _, err := fmt.Sscanf(hm, "%d:%d", &h, &m); err != nil {
		return 0, fmt.Errorf("формат HH:MM: %w", err)
	}
	return h*60 + m, nil
}
