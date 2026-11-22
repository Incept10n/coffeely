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

// Break — перерыв в работе точки ("HH:MM" локального времени точки).
// Несколько перерывов за день — нормально: обед и техпауза.
type Break struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// CapacityConfig — параметры ёмкости точки (jsonb capacity_config).
type CapacityConfig struct {
	WindowMinutes      int    // длина окна производства
	MaxOrdersPerWindow int    // лимит заказов на окно
	WorkStart          string // "HH:MM" локального времени точки
	WorkEnd            string
	Breaks             []Break // перерывы внутри рабочего дня
	AcceptETAMinutes   int     // за сколько минут бариста должен взять заказ
}

// defaultAcceptETA — ETA «бариста возьмёт заказ», если владелец не настроил.
const defaultAcceptETA = 5

// defaults — соло-режим AIK-13: разумные значения без настройки.
func defaultCapacity() CapacityConfig {
	return CapacityConfig{
		WindowMinutes: 5, MaxOrdersPerWindow: 3,
		WorkStart: "08:00", WorkEnd: "20:00",
		AcceptETAMinutes: defaultAcceptETA,
	}
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
		Breaks             *[]struct {
			Start string `json:"start"`
			End   string `json:"end"`
		} `json:"breaks"`
		AcceptETAMinutes *int `json:"accept_eta_minutes"`
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
	if stored.Breaks != nil {
		cfg.Breaks = make([]Break, 0, len(*stored.Breaks))
		for _, b := range *stored.Breaks {
			cfg.Breaks = append(cfg.Breaks, Break{Start: b.Start, End: b.End})
		}
	}
	if stored.AcceptETAMinutes != nil && *stored.AcceptETAMinutes >= 0 && *stored.AcceptETAMinutes <= 180 {
		cfg.AcceptETAMinutes = *stored.AcceptETAMinutes
	}
	return cfg
}

// AcceptETA — сколько времени баристе даётся на то, чтобы взять заказ
// (capacity_config.accept_eta_minutes, дефолт соло-режима).
func AcceptETA(raw []byte) time.Duration {
	return time.Duration(parseCapacity(raw).AcceptETAMinutes) * time.Minute
}

// breakRanges — валидные перерывы в минутах от начала суток. Кривые записи
// (не HH:MM, нулевая длина) игнорируются: точка продолжает работать. Перерыв
// может идти через полночь (23:00–02:00): время всегда идёт вперёд.
type breakRange struct{ start, end int }

func breakRanges(cfg CapacityConfig) []breakRange {
	out := make([]breakRange, 0, len(cfg.Breaks))
	for _, b := range cfg.Breaks {
		start, err1 := parseHM(b.Start)
		end, err2 := parseHM(b.End)
		if err1 != nil || err2 != nil || start >= 1440 || end > 1440 || end == start {
			continue
		}
		out = append(out, breakRange{start: start, end: end})
	}
	return out
}

// dayRange — отрезок суток в минутах от локальной полуночи. Время идёт вперёд:
// конец раньше начала = отрезок через полночь (06:00–01:00 — это 19 часов),
// совпадение = сутки целиком (круглосуточно).
type dayRange struct {
	start, end int
	wrap       bool
	full       bool
}

func newDayRange(start, end int) dayRange {
	switch {
	case end > start:
		return dayRange{start: start, end: end}
	case end < start:
		return dayRange{start: start, end: end, wrap: true}
	default:
		return dayRange{start: start, end: 1440, full: true}
	}
}

// contains — попадают ли минуты суток в отрезок.
func (r dayRange) contains(minutes int) bool {
	switch {
	case r.full:
		return true
	case r.wrap:
		return minutes >= r.start || minutes < r.end
	default:
		return minutes >= r.start && minutes < r.end
	}
}

// segments — отрезок в виде интервалов [начало, конец) в минутах от полуночи
// текущих суток. Для ночного отрезка нужны два: хвост прошлых суток и начало
// следующих — иначе окно сразу после полуночи не увидит вчерашний отрезок.
func (r dayRange) segments() [][2]int {
	switch {
	case r.full:
		return [][2]int{{0, 1440}}
	case r.wrap:
		return [][2]int{{r.start - 1440, r.end}, {r.start, r.end + 1440}}
	default:
		return [][2]int{{r.start, r.end}}
	}
}

// workRange — рабочие часы точки из capacity_config.
func workRange(cfg CapacityConfig) (dayRange, error) {
	start, err := parseHM(cfg.WorkStart)
	if err != nil {
		return dayRange{}, fmt.Errorf("work_start: %w", err)
	}
	end, err := parseHM(cfg.WorkEnd)
	if err != nil {
		return dayRange{}, fmt.Errorf("work_end: %w", err)
	}
	if start >= 1440 || end > 1440 {
		return dayRange{}, fmt.Errorf("часы работы вне суток: %q–%q", cfg.WorkStart, cfg.WorkEnd)
	}
	return newDayRange(start, end), nil
}

// Load — текущая загрузка точки: сколько ждать, если заказать прямо сейчас.
type Load struct {
	// WaitSeconds — работа, которая уже стоит в очереди (сумма prep активных заказов).
	WaitSeconds int `json:"wait_seconds"`
	// ActiveOrders — заказов в работе (paid + in_production).
	ActiveOrders int `json:"active_orders"`
	// IsOpen — точка сейчас принимает заказы (в рабочих часах и не на перерыве).
	IsOpen bool `json:"is_open"`
	// OnBreak — сейчас перерыв; OpensAt — когда он закончится.
	OnBreak bool `json:"on_break"`
	// OpensAt — "HH:MM", когда откроется (только если закрыто).
	OpensAt string `json:"opens_at,omitempty"`
}

// Point — точка выдачи для клиента.
type Point struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	BrandName string          `json:"brand_name"`
	Address   string          `json:"address"`
	Lat       *float64        `json:"lat,omitempty"`
	Lng       *float64        `json:"lng,omitempty"`
	Geo       json.RawMessage `json:"geo"`
	Timezone  string          `json:"timezone"`
	Load      Load            `json:"load"`
}

// Addon — добавка к напитку (сироп, молоко, доп. шот).
type Addon struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	PriceMinor  int64  `json:"price_minor"`
	PrepSeconds int    `json:"prep_seconds"`
}

// MenuItem — позиция меню точки. Addons — добавки именно этого напитка.
type MenuItem struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	PriceMinor  int64   `json:"price_minor"`
	PrepSeconds int     `json:"prep_seconds"`
	Addons      []Addon `json:"addons"`
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
		SELECT p.id, p.title, COALESCE(u.brand_name, ''), COALESCE(p.address, ''),
		       p.lat, p.lng, p.geo, p.timezone, p.capacity_config,
		       COALESCE(q.prep_seconds, 0), COALESCE(q.active_orders, 0)
		FROM points p
		LEFT JOIN users u ON u.id = p.owner_id
		LEFT JOIN LATERAL (
			SELECT COALESCE(SUM((mi.prep_seconds + COALESCE(a.prep_seconds, 0)) * oi.qty), 0)::int AS prep_seconds,
			       COUNT(DISTINCT o.id)::int AS active_orders
			FROM orders o
			JOIN order_items oi ON oi.order_id = o.id
			JOIN menu_items mi ON mi.id = oi.menu_item_id
			LEFT JOIN LATERAL (
				SELECT SUM(oia.prep_seconds)::int AS prep_seconds
				FROM order_item_addons oia WHERE oia.order_item_id = oi.id
			) a ON true
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
		if err := rows.Scan(&p.ID, &p.Title, &p.BrandName, &p.Address, &p.Lat, &p.Lng, &geo, &p.Timezone, &capacityRaw,
			&p.Load.WaitSeconds, &p.Load.ActiveOrders); err != nil {
			return nil, fmt.Errorf("points: scan: %w", err)
		}
		p.Geo = json.RawMessage(geo)
		p.Load.IsOpen, p.Load.OpensAt, p.Load.OnBreak = openState(parseCapacity(capacityRaw), p.Timezone, now)
		out = append(out, p)
	}
	return out, rows.Err()
}

// Menu — доступные позиции точки вместе с добавками каждого товара.
func (s *Store) Menu(ctx context.Context, pointID string) ([]MenuItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, title, price_minor, prep_seconds
		FROM menu_items WHERE point_id = $1 AND is_available ORDER BY title`, pointID)
	if err != nil {
		return nil, fmt.Errorf("points: menu: %w", err)
	}
	defer rows.Close()
	out := []MenuItem{}
	ids := make([]string, 0)
	index := make(map[string]int)
	for rows.Next() {
		var m MenuItem
		if err := rows.Scan(&m.ID, &m.Title, &m.PriceMinor, &m.PrepSeconds); err != nil {
			return nil, fmt.Errorf("points: scan menu: %w", err)
		}
		m.Addons = []Addon{}
		index[m.ID] = len(out)
		out = append(out, m)
		ids = append(ids, m.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return out, nil
	}

	addonRows, err := s.pool.Query(ctx, `
		SELECT menu_item_id, id, title, price_minor, prep_seconds
		FROM addons
		WHERE menu_item_id = ANY($1::uuid[]) AND is_available
		ORDER BY title`, ids)
	if err != nil {
		return nil, fmt.Errorf("points: addons: %w", err)
	}
	defer addonRows.Close()
	for addonRows.Next() {
		var menuItemID string
		var a Addon
		if err := addonRows.Scan(&menuItemID, &a.ID, &a.Title, &a.PriceMinor, &a.PrepSeconds); err != nil {
			return nil, fmt.Errorf("points: scan addon: %w", err)
		}
		if i, ok := index[menuItemID]; ok {
			out[i].Addons = append(out[i].Addons, a)
		}
	}
	return out, addonRows.Err()
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
		SELECT COALESCE(SUM((mi.prep_seconds + COALESCE(a.prep_seconds, 0)) * oi.qty), 0)::int
		FROM orders o
		JOIN order_items oi ON oi.order_id = o.id
		JOIN menu_items mi ON mi.id = oi.menu_item_id
		LEFT JOIN LATERAL (
			SELECT SUM(oia.prep_seconds)::int AS prep_seconds
			FROM order_item_addons oia WHERE oia.order_item_id = oi.id
		) a ON true
		WHERE o.point_id = $1 AND o.status IN ('paid', 'in_production')`, pointID,
	).Scan(&queueSeconds); err != nil {
		return SlotPlan{}, fmt.Errorf("points: queue prep: %w", err)
	}

	work, err := workRange(capacity)
	if err != nil {
		return SlotPlan{}, fmt.Errorf("points: часы работы: %w", err)
	}
	breaks := breakRanges(capacity)

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
		wStart, wEnd, open := windowAt(candidate, loc, work, capacity.WindowMinutes)
		if !open {
			candidate = nextOpening(candidate, loc, work)
			continue
		}
		// Окно должно начинаться не раньше, чем точка освободится: бариста
		// получает полный prep заказа ДО начала окна (ядро AIK-11).
		if wStart.Before(ready) {
			candidate = wEnd
			continue
		}
		// Перерыв: окно не выдаём, ждём конца перерыва (обед, техпауза).
		if bEnd, onBreak := breakOverlapEnd(wStart, wEnd, loc, breaks); onBreak {
			candidate = bEnd
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

// openState — открыта ли точка в момент now (в её таймзоне). Если закрыта,
// возвращает, когда откроется, и признак «это перерыв», чтобы гость видел
// «перерыв до 14:30», а не «закрыто».
func openState(capacity CapacityConfig, tz string, now time.Time) (open bool, opensAt string, onBreak bool) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return false, capacity.WorkStart, false
	}
	work, err := workRange(capacity)
	if err != nil {
		return false, "", false
	}
	local := now.In(loc)
	minutes := local.Hour()*60 + local.Minute()
	if !work.contains(minutes) {
		return false, capacity.WorkStart, false
	}
	for _, b := range breakRanges(capacity) {
		if newDayRange(b.start, b.end).contains(minutes) {
			return false, formatHM(b.end), true
		}
	}
	return true, "", false
}

// breakOverlapEnd — окно [wStart, wEnd) пересекается с перерывом? Тогда
// возвращает момент конца перерыва: следующая попытка планирования — после него.
func breakOverlapEnd(wStart, wEnd time.Time, loc *time.Location, breaks []breakRange) (time.Time, bool) {
	if len(breaks) == 0 {
		return time.Time{}, false
	}
	local := wStart.In(loc)
	dayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	for _, b := range breaks {
		for _, seg := range newDayRange(b.start, b.end).segments() {
			segStart := dayStart.Add(time.Duration(seg[0]) * time.Minute)
			segEnd := dayStart.Add(time.Duration(seg[1]) * time.Minute)
			if wStart.Before(segEnd) && wEnd.After(segStart) {
				return segEnd, true
			}
		}
	}
	return time.Time{}, false
}

// windowAt — окно сетки, в которое попадает момент ts. open=false — ts вне
// рабочих часов (сетка начинается с открытия смены).
func windowAt(ts time.Time, loc *time.Location, work dayRange, windowMinutes int) (start, end time.Time, open bool) {
	if windowMinutes <= 0 {
		windowMinutes = 5
	}
	local := ts.In(loc)
	minutes := local.Hour()*60 + local.Minute()
	if !work.contains(minutes) {
		return time.Time{}, time.Time{}, false
	}
	dayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	base := dayStart.Add(time.Duration(work.start) * time.Minute)
	// Сетка начинается с открытия. Если открытие сегодня ещё впереди, момент
	// принадлежит сетке, начавшейся вчера (ночное окно и «круглосуточно»).
	if base.After(ts) {
		base = base.AddDate(0, 0, -1)
	}
	wStart := gridWindow(ts, base, windowMinutes)
	return wStart, wStart.Add(time.Duration(windowMinutes) * time.Minute), true
}

// gridWindow — начало окна сетки длиной windowMinutes, в которое попадает ts.
func gridWindow(ts, base time.Time, windowMinutes int) time.Time {
	if delta := int(ts.Sub(base) / time.Minute); delta > 0 {
		return base.Add(time.Duration((delta/windowMinutes)*windowMinutes) * time.Minute)
	}
	return base
}

// nextOpening — когда откроется после ts. Время идёт вперёд: если сегодняшнее
// открытие уже прошло (например, точка закрылась в 01:00), ждём завтрашнее.
func nextOpening(ts time.Time, loc *time.Location, work dayRange) time.Time {
	local := ts.In(loc)
	dayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	today := dayStart.Add(time.Duration(work.start) * time.Minute)
	if local.Before(today) {
		return today
	}
	return today.AddDate(0, 0, 1)
}

func parseHM(hm string) (int, error) {
	var h, m int
	if _, err := fmt.Sscanf(hm, "%d:%d", &h, &m); err != nil {
		return 0, fmt.Errorf("формат HH:MM: %w", err)
	}
	return h*60 + m, nil
}

// formatHM — минуты от начала суток обратно в "HH:MM".
func formatHM(minutes int) string {
	if minutes < 0 {
		minutes = 0
	}
	return fmt.Sprintf("%02d:%02d", (minutes/60)%24, minutes%60)
}
