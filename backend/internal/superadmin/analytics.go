// Package superadmin — внутренняя админка создателей бизнеса: одна ссылка
// /admin на сайте, вход по общему секрету ADMIN_CRM_SECRET, только чтение
// аналитики по всей платформе (владельцы, точки, заказы, деньги, дедлайны).
//
// Это не кабинет владельца (internal/admin): кабинет — для клиентов бизнеса,
// здесь — сводная картина для нас.
package superadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Автозаполнение пропущенных дат и срезов делается в Go: SQL отдаёт только
// непустые группы, а фронтенду нужны ровные ряды (24 часа, N дней).

// Store — SQL-аналитика по всей базе.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Analytics — полный срез для админки за окно последних N дней.
type Analytics struct {
	GeneratedAt time.Time  `json:"generated_at"`
	Window      Window     `json:"window"`
	Totals      Totals     `json:"totals"`
	Daily       []Daily    `json:"daily"`
	Funnel      []Funnel   `json:"funnel"`
	Hours       []Bucket   `json:"hours"`
	Weekdays    []Bucket   `json:"weekdays"`
	Points      []Point    `json:"points"`
	Owners      []Owner    `json:"owners"`
	TopItems    []TopItem  `json:"top_items"`
	Baristas    []Barista  `json:"baristas"`
	Activation  Activation `json:"activation"`
}

// Window — границы окна (московские даты: бизнес живёт в РФ).
type Window struct {
	Days int    `json:"days"`
	From string `json:"from"`
	To   string `json:"to"`
}

// Totals — сводные показатели платформы.
type Totals struct {
	Owners       int64 `json:"owners"`
	OwnersNew    int64 `json:"owners_new"`
	Clients      int64 `json:"clients"`
	ClientsNew   int64 `json:"clients_new"`
	Baristas     int64 `json:"baristas"`
	Points       int64 `json:"points"`
	PointsActive int64 `json:"points_active"`

	Orders       int64 `json:"orders"`
	OrdersWindow int64 `json:"orders_window"`
	OrdersToday  int64 `json:"orders_today"`
	OrdersPrev   int64 `json:"orders_prev"`
	ActiveOrders int64 `json:"active_orders"`
	OverdueNow   int64 `json:"overdue_now"`

	RevenueMinor       int64   `json:"revenue_minor"`        // выданные, всё время
	RevenueWindowMinor int64   `json:"revenue_window_minor"` // выданные, окно
	RevenuePrevMinor   int64   `json:"revenue_prev_minor"`   // выданные, прошлое окно
	BookedWindowMinor  int64   `json:"booked_window_minor"`  // окно, без отмен
	AOVMinor           int64   `json:"aov_minor"`            // средний чек, окно
	OrdersGrowthPct    float64 `json:"orders_growth_pct"`
	RevenueGrowthPct   float64 `json:"revenue_growth_pct"`

	GivenOutWindow  int64   `json:"given_out_window"`
	CancelledWindow int64   `json:"cancelled_window"`
	CancelRatePct   float64 `json:"cancel_rate_pct"`

	OnTimeWindow       int64   `json:"on_time_window"`
	MissedWindow       int64   `json:"missed_window"`
	OnTimePct          float64 `json:"on_time_pct"`
	AvgLatenessSeconds int64   `json:"avg_lateness_seconds"`

	AvgStartDelaySeconds  int64 `json:"avg_start_delay_seconds"`  // создан → в работу
	AvgMakeSeconds        int64 `json:"avg_make_seconds"`         // в работу → готов
	AvgPickupDelaySeconds int64 `json:"avg_pickup_delay_seconds"` // готов → выдан
	AvgLeadSeconds        int64 `json:"avg_lead_seconds"`         // создан → выдан

	RepeatClients     int64   `json:"repeat_clients"`
	ClientsWithOrders int64   `json:"clients_with_orders"`
	RepeatRatePct     float64 `json:"repeat_rate_pct"`
}

// Daily — день ряда: заказы, деньги, регистрации.
type Daily struct {
	Date         string `json:"date"`
	Orders       int64  `json:"orders"`
	GivenOut     int64  `json:"given_out"`
	Cancelled    int64  `json:"cancelled"`
	InFlight     int64  `json:"in_flight"`
	RevenueMinor int64  `json:"revenue_minor"`
	NewClients   int64  `json:"new_clients"`
	NewOwners    int64  `json:"new_owners"`
}

// Funnel — распределение заказов окна по статусам.
type Funnel struct {
	Status     string `json:"status"`
	Count      int64  `json:"count"`
	TotalMinor int64  `json:"total_minor"`
}

// Bucket — срез по часу (0–23) или дню недели (1–7, Пн=1).
type Bucket struct {
	Key    int   `json:"key"`
	Orders int64 `json:"orders"`
}

// Point — показатели точки.
type Point struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Address    string `json:"address"`
	OwnerEmail string `json:"owner_email"`
	IsActive   bool   `json:"is_active"`

	OrdersWindow       int64 `json:"orders_window"`
	OrdersTotal        int64 `json:"orders_total"`
	GivenOutWindow     int64 `json:"given_out_window"`
	CancelledWindow    int64 `json:"cancelled_window"`
	RevenueWindowMinor int64 `json:"revenue_window_minor"`
	AOVMinor           int64 `json:"aov_minor"`

	OnTime    int64   `json:"on_time"`
	Missed    int64   `json:"missed"`
	OnTimePct float64 `json:"on_time_pct"`

	ActiveOrders int64 `json:"active_orders"`
	OverdueNow   int64 `json:"overdue_now"`
	Baristas     int64 `json:"baristas"`

	LastOrderAt *string `json:"last_order_at"`

	// Ёмкость и загрузка: сколько окон в рабочий день × лимит на окно.
	WindowMinutes      int      `json:"window_minutes"`
	MaxOrdersPerWindow int      `json:"max_orders_per_window"`
	WorkStart          string   `json:"work_start"`
	WorkEnd            string   `json:"work_end"`
	CapacityPerDay     int64    `json:"capacity_per_day"`
	UtilizationPct     *float64 `json:"utilization_pct"`
}

// Owner — владелец кофейни и его вклад.
type Owner struct {
	Email        string  `json:"email"`
	CreatedAt    string  `json:"created_at"`
	Points       int64   `json:"points"`
	Baristas     int64   `json:"baristas"`
	OrdersWindow int64   `json:"orders_window"`
	RevenueMinor int64   `json:"revenue_minor"`
	LastOrderAt  *string `json:"last_order_at"`
}

// TopItem — позиция меню по выручке.
type TopItem struct {
	Title        string `json:"title"`
	Qty          int64  `json:"qty"`
	RevenueMinor int64  `json:"revenue_minor"`
}

// Barista — активность рабочего аккаунта.
type Barista struct {
	Email          string `json:"email"`
	PointTitle     string `json:"point_title"`
	Closed         int64  `json:"closed"`
	AvgLeadSeconds int64  `json:"avg_lead_seconds"`
}

// Activation — воронка «зарегистрировался → начал работать».
type Activation struct {
	ClientsNoOrders  int64 `json:"clients_no_orders"`
	OwnersNoPoints   int64 `json:"owners_no_points"`
	OwnersNoBaristas int64 `json:"owners_no_baristas"`
	PointsNoOrders   int64 `json:"points_no_orders"`
}

const moscowTZ = "Europe/Moscow"

// Analytics собирает все срезы. days уже проверен (1..365).
func (s *Store) Analytics(ctx context.Context, days int, now time.Time) (Analytics, error) {
	loc, err := time.LoadLocation(moscowTZ)
	if err != nil {
		return Analytics{}, fmt.Errorf("superadmin: tz: %w", err)
	}
	localNow := now.In(loc)
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
	from := today.AddDate(0, 0, -(days - 1))

	a := Analytics{
		GeneratedAt: now.UTC(),
		Window: Window{
			Days: days,
			From: from.Format("2006-01-02"),
			To:   today.Format("2006-01-02"),
		},
	}

	if err := s.totals(ctx, from, from.AddDate(0, 0, -days), &a); err != nil {
		return Analytics{}, err
	}
	if err := s.deadlines(ctx, from, &a); err != nil {
		return Analytics{}, err
	}
	if err := s.stageDurations(ctx, from, &a); err != nil {
		return Analytics{}, err
	}
	if err := s.retention(ctx, &a); err != nil {
		return Analytics{}, err
	}
	daily, err := s.daily(ctx, from, days, loc)
	if err != nil {
		return Analytics{}, err
	}
	a.Daily = daily
	if err := s.usersDaily(ctx, from, a.Daily); err != nil {
		return Analytics{}, err
	}
	if a.Funnel, err = s.funnel(ctx, from); err != nil {
		return Analytics{}, err
	}
	if a.Hours, err = s.hours(ctx, from); err != nil {
		return Analytics{}, err
	}
	if a.Weekdays, err = s.weekdays(ctx, from); err != nil {
		return Analytics{}, err
	}
	if a.Points, err = s.points(ctx, from, days); err != nil {
		return Analytics{}, err
	}
	if a.Owners, err = s.owners(ctx, from); err != nil {
		return Analytics{}, err
	}
	if a.TopItems, err = s.topItems(ctx, from); err != nil {
		return Analytics{}, err
	}
	if a.Baristas, err = s.baristas(ctx, from); err != nil {
		return Analytics{}, err
	}
	if a.Activation, err = s.activation(ctx); err != nil {
		return Analytics{}, err
	}
	return a, nil
}

func (s *Store) totals(ctx context.Context, from, prevFrom time.Time, a *Analytics) error {
	t := &a.Totals
	err := s.pool.QueryRow(ctx, `
		SELECT
		 (SELECT count(*) FROM users WHERE role = 'owner'),
		 (SELECT count(*) FROM users WHERE role = 'owner' AND created_at >= $1),
		 (SELECT count(*) FROM users WHERE role = 'client'),
		 (SELECT count(*) FROM users WHERE role = 'client' AND created_at >= $1),
		 (SELECT count(*) FROM users WHERE role = 'barista'),
		 (SELECT count(*) FROM points),
		 (SELECT count(*) FROM points WHERE is_active),
		 (SELECT count(*) FROM orders),
		 (SELECT count(*) FROM orders WHERE created_at >= $1),
		 (SELECT count(*) FROM orders WHERE created_at >=
		     date_trunc('day', now() AT TIME ZONE 'Europe/Moscow') AT TIME ZONE 'Europe/Moscow'),
		 (SELECT count(*) FROM orders WHERE created_at >= $2 AND created_at < $1),
		 (SELECT count(*) FROM orders WHERE status IN ('paid', 'in_production', 'ready')),
		 (SELECT count(*) FROM orders WHERE status IN ('paid', 'in_production', 'ready')
		     AND deadline_at < now()),
		 (SELECT coalesce(sum(total_minor), 0) FROM orders WHERE status = 'given_out'),
		 (SELECT coalesce(sum(total_minor), 0) FROM orders
		     WHERE status = 'given_out' AND created_at >= $1),
		 (SELECT coalesce(sum(total_minor), 0) FROM orders
		     WHERE status = 'given_out' AND created_at >= $2 AND created_at < $1),
		 (SELECT coalesce(sum(total_minor), 0) FROM orders
		     WHERE created_at >= $1 AND status <> 'cancelled'),
		 (SELECT coalesce(round(avg(total_minor)), 0)::bigint FROM orders
		     WHERE status = 'given_out' AND created_at >= $1),
		 (SELECT count(*) FROM orders WHERE status = 'given_out' AND created_at >= $1),
		 (SELECT count(*) FROM orders WHERE status = 'cancelled' AND created_at >= $1)`,
		from, prevFrom,
	).Scan(
		&t.Owners, &t.OwnersNew, &t.Clients, &t.ClientsNew, &t.Baristas,
		&t.Points, &t.PointsActive,
		&t.Orders, &t.OrdersWindow, &t.OrdersToday, &t.OrdersPrev,
		&t.ActiveOrders, &t.OverdueNow,
		&t.RevenueMinor, &t.RevenueWindowMinor, &t.RevenuePrevMinor,
		&t.BookedWindowMinor, &t.AOVMinor,
		&t.GivenOutWindow, &t.CancelledWindow,
	)
	if err != nil {
		return fmt.Errorf("superadmin: totals: %w", err)
	}
	if denom := t.GivenOutWindow + t.CancelledWindow; denom > 0 {
		t.CancelRatePct = round1(float64(t.CancelledWindow) / float64(denom) * 100)
	}
	if t.OrdersPrev > 0 {
		t.OrdersGrowthPct = round1(float64(t.OrdersWindow-t.OrdersPrev) /
			float64(t.OrdersPrev) * 100)
	}
	if t.RevenuePrevMinor > 0 {
		t.RevenueGrowthPct = round1(float64(t.RevenueWindowMinor-t.RevenuePrevMinor) /
			float64(t.RevenuePrevMinor) * 100)
	}
	return nil
}

func (s *Store) deadlines(ctx context.Context, from time.Time, a *Analytics) error {
	var avgLateness float64
	err := s.pool.QueryRow(ctx, `
		WITH outs AS (
			SELECT DISTINCT ON (ev.order_id)
			       ev.order_id, ev.at, o.deadline_at
			FROM order_events ev
			JOIN orders o ON o.id = ev.order_id
			WHERE ev.event = 'given_out' AND ev.at >= $1
			ORDER BY ev.order_id, ev.at
		)
		SELECT count(*) FILTER (WHERE at <= deadline_at),
		       count(*) FILTER (WHERE at > deadline_at),
		       coalesce(avg(extract(epoch FROM (at - deadline_at))) FILTER (WHERE at > deadline_at), 0)
		FROM outs`, from,
	).Scan(&a.Totals.OnTimeWindow, &a.Totals.MissedWindow, &avgLateness)
	if err != nil {
		return fmt.Errorf("superadmin: deadlines: %w", err)
	}
	if total := a.Totals.OnTimeWindow + a.Totals.MissedWindow; total > 0 {
		a.Totals.OnTimePct = round1(float64(a.Totals.OnTimeWindow) / float64(total) * 100)
	}
	a.Totals.AvgLatenessSeconds = int64(avgLateness)
	return nil
}

func (s *Store) stageDurations(ctx context.Context, from time.Time, a *Analytics) error {
	var startDelay, makeDur, pickupDelay, lead float64
	err := s.pool.QueryRow(ctx, `
		WITH t AS (
			SELECT o.id, o.created_at,
			       min(ev.at) FILTER (WHERE ev.event = 'in_production') AS started,
			       min(ev.at) FILTER (WHERE ev.event = 'ready')         AS ready,
			       min(ev.at) FILTER (WHERE ev.event = 'given_out')     AS done
			FROM orders o
			JOIN order_events ev ON ev.order_id = o.id
			WHERE o.created_at >= $1
			GROUP BY o.id, o.created_at
		)
		SELECT
		 coalesce(avg(extract(epoch FROM (started - created_at)))
		          FILTER (WHERE started IS NOT NULL AND started >= created_at), 0),
		 coalesce(avg(extract(epoch FROM (ready - started)))
		          FILTER (WHERE ready IS NOT NULL AND started IS NOT NULL AND ready >= started), 0),
		 coalesce(avg(extract(epoch FROM (done - ready)))
		          FILTER (WHERE done IS NOT NULL AND ready IS NOT NULL AND done >= ready), 0),
		 coalesce(avg(extract(epoch FROM (done - created_at)))
		          FILTER (WHERE done IS NOT NULL AND done >= created_at), 0)
		FROM t`, from,
	).Scan(&startDelay, &makeDur, &pickupDelay, &lead)
	if err != nil {
		return fmt.Errorf("superadmin: stage durations: %w", err)
	}
	a.Totals.AvgStartDelaySeconds = int64(startDelay)
	a.Totals.AvgMakeSeconds = int64(makeDur)
	a.Totals.AvgPickupDelaySeconds = int64(pickupDelay)
	a.Totals.AvgLeadSeconds = int64(lead)
	return nil
}

func (s *Store) retention(ctx context.Context, a *Analytics) error {
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE c > 1),
		       count(*)
		FROM (
			SELECT client_id, count(*) AS c
			FROM orders WHERE status = 'given_out'
			GROUP BY client_id
		) t`,
	).Scan(&a.Totals.RepeatClients, &a.Totals.ClientsWithOrders)
	if err != nil {
		return fmt.Errorf("superadmin: retention: %w", err)
	}
	if a.Totals.ClientsWithOrders > 0 {
		a.Totals.RepeatRatePct = round1(float64(a.Totals.RepeatClients) /
			float64(a.Totals.ClientsWithOrders) * 100)
	}
	return nil
}

func (s *Store) daily(ctx context.Context, from time.Time, days int, loc *time.Location) ([]Daily, error) {
	byDate := make(map[string]Daily, days)
	rows, err := s.pool.Query(ctx, `
		SELECT to_char((o.created_at AT TIME ZONE p.timezone)::date, 'YYYY-MM-DD') AS d,
		       count(*),
		       count(*) FILTER (WHERE o.status = 'given_out'),
		       count(*) FILTER (WHERE o.status = 'cancelled'),
		       count(*) FILTER (WHERE o.status IN ('paid', 'in_production', 'ready')),
		       coalesce(sum(o.total_minor) FILTER (WHERE o.status = 'given_out'), 0)
		FROM orders o
		JOIN points p ON p.id = o.point_id
		WHERE o.created_at >= $1
		GROUP BY 1
		ORDER BY 1`, from)
	if err != nil {
		return nil, fmt.Errorf("superadmin: daily: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var d Daily
		if err := rows.Scan(&d.Date, &d.Orders, &d.GivenOut, &d.Cancelled, &d.InFlight,
			&d.RevenueMinor); err != nil {
			return nil, fmt.Errorf("superadmin: daily scan: %w", err)
		}
		byDate[d.Date] = d
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]Daily, 0, days)
	for i := 0; i < days; i++ {
		date := from.In(loc).AddDate(0, 0, i).Format("2006-01-02")
		if d, ok := byDate[date]; ok {
			out = append(out, d)
		} else {
			out = append(out, Daily{Date: date})
		}
	}
	return out, nil
}

// usersDaily добавляет в ряд регистрации клиентов и владельцев.
func (s *Store) usersDaily(ctx context.Context, from time.Time, daily []Daily) error {
	idx := make(map[string]int, len(daily))
	for i := range daily {
		idx[daily[i].Date] = i
	}
	rows, err := s.pool.Query(ctx, `
		SELECT to_char((created_at AT TIME ZONE 'Europe/Moscow')::date, 'YYYY-MM-DD'), role, count(*)
		FROM users
		WHERE created_at >= $1 AND role IN ('client', 'owner')
		GROUP BY 1, 2`, from)
	if err != nil {
		return fmt.Errorf("superadmin: users daily: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var date, role string
		var n int64
		if err := rows.Scan(&date, &role, &n); err != nil {
			return fmt.Errorf("superadmin: users daily scan: %w", err)
		}
		i, ok := idx[date]
		if !ok {
			continue
		}
		if role == "client" {
			daily[i].NewClients = n
		} else {
			daily[i].NewOwners = n
		}
	}
	return rows.Err()
}

func (s *Store) funnel(ctx context.Context, from time.Time) ([]Funnel, error) {
	byStatus := make(map[string]Funnel, 6)
	rows, err := s.pool.Query(ctx, `
		SELECT status, count(*), coalesce(sum(total_minor), 0)
		FROM orders WHERE created_at >= $1
		GROUP BY status`, from)
	if err != nil {
		return nil, fmt.Errorf("superadmin: funnel: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var f Funnel
		if err := rows.Scan(&f.Status, &f.Count, &f.TotalMinor); err != nil {
			return nil, fmt.Errorf("superadmin: funnel scan: %w", err)
		}
		byStatus[f.Status] = f
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	order := []string{"paid", "in_production", "ready", "given_out", "cancelled"}
	out := make([]Funnel, 0, len(order))
	for _, st := range order {
		f, ok := byStatus[st]
		if !ok {
			f = Funnel{Status: st}
		}
		out = append(out, f)
	}
	return out, nil
}

func (s *Store) hours(ctx context.Context, from time.Time) ([]Bucket, error) {
	byHour := make(map[int]int64, 24)
	rows, err := s.pool.Query(ctx, `
		SELECT extract(hour FROM (o.created_at AT TIME ZONE p.timezone))::int AS h,
		       count(*)
		FROM orders o JOIN points p ON p.id = o.point_id
		WHERE o.created_at >= $1
		GROUP BY 1`, from)
	if err != nil {
		return nil, fmt.Errorf("superadmin: hours: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var h int
		var n int64
		if err := rows.Scan(&h, &n); err != nil {
			return nil, fmt.Errorf("superadmin: hours scan: %w", err)
		}
		byHour[h] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Bucket, 24)
	for h := 0; h < 24; h++ {
		out[h] = Bucket{Key: h, Orders: byHour[h]}
	}
	return out, nil
}

func (s *Store) weekdays(ctx context.Context, from time.Time) ([]Bucket, error) {
	byDay := make(map[int]int64, 7)
	rows, err := s.pool.Query(ctx, `
		SELECT extract(isodow FROM (o.created_at AT TIME ZONE p.timezone))::int AS d,
		       count(*)
		FROM orders o JOIN points p ON p.id = o.point_id
		WHERE o.created_at >= $1
		GROUP BY 1`, from)
	if err != nil {
		return nil, fmt.Errorf("superadmin: weekdays: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var d int
		var n int64
		if err := rows.Scan(&d, &n); err != nil {
			return nil, fmt.Errorf("superadmin: weekdays scan: %w", err)
		}
		byDay[d] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Bucket, 7)
	for d := 1; d <= 7; d++ {
		out[d-1] = Bucket{Key: d, Orders: byDay[d]}
	}
	return out, nil
}

func (s *Store) points(ctx context.Context, from time.Time, days int) ([]Point, error) {
	// Конфиг и владелец — отдельным запросом: агрегаты по заказам живут в CTE.
	rows, err := s.pool.Query(ctx, `
		WITH agg AS (
			SELECT p.id,
			       count(o.id) FILTER (WHERE o.created_at >= $1)                          AS orders_window,
			       count(o.id)                                                            AS orders_total,
			       count(o.id) FILTER (WHERE o.created_at >= $1 AND o.status = 'given_out') AS given_out_window,
			       count(o.id) FILTER (WHERE o.created_at >= $1 AND o.status = 'cancelled') AS cancelled_window,
			       coalesce(sum(o.total_minor) FILTER (WHERE o.created_at >= $1 AND o.status = 'given_out'), 0) AS revenue_window,
			       coalesce(round(avg(o.total_minor) FILTER (WHERE o.created_at >= $1 AND o.status = 'given_out')), 0)::bigint AS aov,
			       count(o.id) FILTER (WHERE o.status IN ('paid', 'in_production', 'ready')) AS active_orders,
			       count(o.id) FILTER (WHERE o.status IN ('paid', 'in_production', 'ready')
			                            AND o.deadline_at < now())                        AS overdue_now,
			       max(o.created_at)                                                     AS last_order_at
			FROM points p
			LEFT JOIN orders o ON o.point_id = p.id
			GROUP BY p.id
		),
		outs AS (
			SELECT DISTINCT ON (ev.order_id)
			       ev.order_id, o.point_id, ev.at, o.deadline_at
			FROM order_events ev
			JOIN orders o ON o.id = ev.order_id
			WHERE ev.event = 'given_out' AND ev.at >= $1
			ORDER BY ev.order_id, ev.at
		),
		deadlines AS (
			SELECT point_id,
			       count(*) FILTER (WHERE at <= deadline_at) AS on_time,
			       count(*) FILTER (WHERE at > deadline_at)  AS missed
			FROM outs GROUP BY point_id
		)
		SELECT p.id, p.title, coalesce(p.address, ''), coalesce(u.email, ''), p.is_active,
		       coalesce(p.capacity_config::text, '{}'),
		       coalesce(a.orders_window, 0), coalesce(a.orders_total, 0),
		       coalesce(a.given_out_window, 0), coalesce(a.cancelled_window, 0),
		       coalesce(a.revenue_window, 0), coalesce(a.aov, 0),
		       coalesce(a.active_orders, 0), coalesce(a.overdue_now, 0),
		       a.last_order_at,
		       coalesce(d.on_time, 0), coalesce(d.missed, 0),
		       (SELECT count(*) FROM barista_points bp WHERE bp.point_id = p.id)
		FROM points p
		LEFT JOIN users u ON u.id = p.owner_id
		LEFT JOIN agg a ON a.id = p.id
		LEFT JOIN deadlines d ON d.point_id = p.id
		ORDER BY p.created_at`, from)
	if err != nil {
		return nil, fmt.Errorf("superadmin: points: %w", err)
	}
	defer rows.Close()

	list := []Point{}
	for rows.Next() {
		var p Point
		var capacityRaw string
		var lastAt *time.Time
		if err := rows.Scan(&p.ID, &p.Title, &p.Address, &p.OwnerEmail, &p.IsActive,
			&capacityRaw, &p.OrdersWindow, &p.OrdersTotal, &p.GivenOutWindow,
			&p.CancelledWindow, &p.RevenueWindowMinor, &p.AOVMinor,
			&p.ActiveOrders, &p.OverdueNow, &lastAt,
			&p.OnTime, &p.Missed, &p.Baristas); err != nil {
			return nil, fmt.Errorf("superadmin: points scan: %w", err)
		}
		if p.OnTime+p.Missed > 0 {
			p.OnTimePct = round1(float64(p.OnTime) / float64(p.OnTime+p.Missed) * 100)
		}
		if lastAt != nil {
			s := lastAt.In(time.FixedZone("MSK", 3*60*60)).Format("2006-01-02 15:04")
			p.LastOrderAt = &s
		}
		applyCapacity(&p, capacityRaw, days)
		list = append(list, p)
	}
	return list, rows.Err()
}

func (s *Store) owners(ctx context.Context, from time.Time) ([]Owner, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.email, u.created_at::text,
		       (SELECT count(*) FROM points p WHERE p.owner_id = u.id),
		       (SELECT count(*) FROM barista_points bp
		          JOIN points p ON p.id = bp.point_id WHERE p.owner_id = u.id),
		       coalesce((SELECT count(*) FROM orders o
		          JOIN points p ON p.id = o.point_id
		          WHERE p.owner_id = u.id AND o.created_at >= $1), 0),
		       coalesce((SELECT sum(o.total_minor) FROM orders o
		          JOIN points p ON p.id = o.point_id
		          WHERE p.owner_id = u.id AND o.status = 'given_out'), 0),
		       (SELECT max(o.created_at) FROM orders o
		          JOIN points p ON p.id = o.point_id WHERE p.owner_id = u.id)
		FROM users u
		WHERE u.role = 'owner'
		ORDER BY u.created_at DESC
		LIMIT 200`, from)
	if err != nil {
		return nil, fmt.Errorf("superadmin: owners: %w", err)
	}
	defer rows.Close()

	list := []Owner{}
	for rows.Next() {
		var o Owner
		var lastAt *time.Time
		if err := rows.Scan(&o.Email, &o.CreatedAt, &o.Points, &o.Baristas,
			&o.OrdersWindow, &o.RevenueMinor, &lastAt); err != nil {
			return nil, fmt.Errorf("superadmin: owners scan: %w", err)
		}
		if lastAt != nil {
			s := lastAt.In(time.FixedZone("MSK", 3*60*60)).Format("2006-01-02 15:04")
			o.LastOrderAt = &s
		}
		list = append(list, o)
	}
	return list, rows.Err()
}

func (s *Store) topItems(ctx context.Context, from time.Time) ([]TopItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT mi.title, sum(oi.qty)::bigint, sum(oi.qty * oi.price_minor)::bigint
		FROM order_items oi
		JOIN orders o ON o.id = oi.order_id
		JOIN menu_items mi ON mi.id = oi.menu_item_id
		WHERE o.created_at >= $1 AND o.status <> 'cancelled'
		GROUP BY mi.title
		ORDER BY 3 DESC, 2 DESC
		LIMIT 12`, from)
	if err != nil {
		return nil, fmt.Errorf("superadmin: top items: %w", err)
	}
	defer rows.Close()

	list := []TopItem{}
	for rows.Next() {
		var it TopItem
		if err := rows.Scan(&it.Title, &it.Qty, &it.RevenueMinor); err != nil {
			return nil, fmt.Errorf("superadmin: top items scan: %w", err)
		}
		list = append(list, it)
	}
	return list, rows.Err()
}

func (s *Store) baristas(ctx context.Context, from time.Time) ([]Barista, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.email, coalesce(p.title, ''),
		       count(DISTINCT ev.order_id)::bigint,
		       coalesce(avg(extract(epoch FROM (ev.at - o.created_at))), 0)::bigint
		FROM order_events ev
		JOIN orders o ON o.id = ev.order_id
		JOIN users u ON u.id = ev.actor_id
		LEFT JOIN barista_points bp ON bp.user_id = u.id
		LEFT JOIN points p ON p.id = bp.point_id
		WHERE ev.event = 'given_out' AND ev.at >= $1
		GROUP BY u.email, p.title
		ORDER BY 3 DESC, 1
		LIMIT 20`, from)
	if err != nil {
		return nil, fmt.Errorf("superadmin: baristas: %w", err)
	}
	defer rows.Close()

	list := []Barista{}
	for rows.Next() {
		var b Barista
		if err := rows.Scan(&b.Email, &b.PointTitle, &b.Closed, &b.AvgLeadSeconds); err != nil {
			return nil, fmt.Errorf("superadmin: baristas scan: %w", err)
		}
		list = append(list, b)
	}
	return list, rows.Err()
}

func (s *Store) activation(ctx context.Context) (Activation, error) {
	var a Activation
	err := s.pool.QueryRow(ctx, `
		SELECT
		 (SELECT count(*) FROM users u WHERE u.role = 'client'
		    AND NOT EXISTS (SELECT 1 FROM orders o WHERE o.client_id = u.id)),
		 (SELECT count(*) FROM users u WHERE u.role = 'owner'
		    AND NOT EXISTS (SELECT 1 FROM points p WHERE p.owner_id = u.id)),
		 (SELECT count(*) FROM users u WHERE u.role = 'owner'
		    AND NOT EXISTS (SELECT 1 FROM barista_points bp
		                      JOIN points p ON p.id = bp.point_id WHERE p.owner_id = u.id)),
		 (SELECT count(*) FROM points p
		    WHERE NOT EXISTS (SELECT 1 FROM orders o WHERE o.point_id = p.id))`,
	).Scan(&a.ClientsNoOrders, &a.OwnersNoPoints, &a.OwnersNoBaristas, &a.PointsNoOrders)
	if err != nil {
		return a, fmt.Errorf("superadmin: activation: %w", err)
	}
	return a, nil
}

type capacityConfig struct {
	WindowMinutes      int    `json:"window_minutes"`
	MaxOrdersPerWindow int    `json:"max_orders_per_window"`
	WorkStart          string `json:"work_start"`
	WorkEnd            string `json:"work_end"`
}

// applyCapacity считает ёмкость и загрузку точки: сетка окон рабочего дня ×
// лимит заказов на окно. Пустой конфиг = дефолты points (5 мин / 3 заказа).
func applyCapacity(p *Point, raw string, days int) {
	cfg := capacityConfig{
		WindowMinutes: 5, MaxOrdersPerWindow: 3,
		WorkStart: "08:00", WorkEnd: "20:00",
	}
	_ = json.Unmarshal([]byte(raw), &cfg)
	if cfg.WindowMinutes <= 0 {
		cfg.WindowMinutes = 5
	}
	if cfg.MaxOrdersPerWindow <= 0 {
		cfg.MaxOrdersPerWindow = 3
	}
	if cfg.WorkStart == "" {
		cfg.WorkStart = "08:00"
	}
	if cfg.WorkEnd == "" {
		cfg.WorkEnd = "20:00"
	}
	p.WindowMinutes = cfg.WindowMinutes
	p.MaxOrdersPerWindow = cfg.MaxOrdersPerWindow
	p.WorkStart = cfg.WorkStart
	p.WorkEnd = cfg.WorkEnd
	p.CapacityPerDay = 0

	start, err1 := parseHM(cfg.WorkStart)
	end, err2 := parseHM(cfg.WorkEnd)
	if err1 != nil || err2 != nil || start >= 1440 || end > 1440 {
		return
	}
	// Время идёт вперёд: конец раньше начала — смена через полночь, совпадение —
	// круглосуточно.
	length := end - start
	if length <= 0 {
		length += 1440
	}
	windows := length / cfg.WindowMinutes
	if windows <= 0 {
		return
	}
	p.CapacityPerDay = int64(windows * cfg.MaxOrdersPerWindow)
	if days <= 0 || p.CapacityPerDay <= 0 {
		return
	}
	util := float64(p.OrdersWindow) / float64(p.CapacityPerDay*int64(days)) * 100
	util = round1(util)
	p.UtilizationPct = &util
}

func parseHM(hm string) (int, error) {
	var h, m int
	if _, err := fmt.Sscanf(strings.TrimSpace(hm), "%d:%d", &h, &m); err != nil {
		return 0, fmt.Errorf("формат HH:MM: %w", err)
	}
	if h < 0 || h > 24 || m < 0 || m > 59 {
		return 0, fmt.Errorf("часы/минуты вне диапазона: %s", hm)
	}
	return h*60 + m, nil
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}
