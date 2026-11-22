package points

import (
	"reflect"
	"testing"
	"time"
)

func TestParseCapacity(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want CapacityConfig
	}{
		{"пусто — дефолты", "", defaultCapacity()},
		{"{} — дефолты", "{}", defaultCapacity()},
		{"битый json — дефолты", "{not json", defaultCapacity()},
		{"переопределение лимита", `{"max_orders_per_window":1}`, CapacityConfig{WindowMinutes: 5, MaxOrdersPerWindow: 1, WorkStart: "08:00", WorkEnd: "20:00", AcceptETAMinutes: defaultAcceptETA}},
		{"переопределение окна", `{"window_minutes":10}`, CapacityConfig{WindowMinutes: 10, MaxOrdersPerWindow: 3, WorkStart: "08:00", WorkEnd: "20:00", AcceptETAMinutes: defaultAcceptETA}},
		{"нули игнорируются", `{"window_minutes":0,"max_orders_per_window":0}`, defaultCapacity()},
		{"часы работы", `{"work_start":"07:30","work_end":"22:00"}`, CapacityConfig{WindowMinutes: 5, MaxOrdersPerWindow: 3, WorkStart: "07:30", WorkEnd: "22:00", AcceptETAMinutes: defaultAcceptETA}},
		{"перерыв и ETA", `{"breaks":[{"start":"13:00","end":"13:30"}],"accept_eta_minutes":8}`, CapacityConfig{WindowMinutes: 5, MaxOrdersPerWindow: 3, WorkStart: "08:00", WorkEnd: "20:00", Breaks: []Break{{Start: "13:00", End: "13:30"}}, AcceptETAMinutes: 8}},
		{"ETA вне диапазона — дефолт", `{"accept_eta_minutes":999}`, defaultCapacity()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCapacity([]byte(tt.raw))
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseCapacity(%q) = %+v, ожидали %+v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestParseHM(t *testing.T) {
	if got, err := parseHM("08:30"); err != nil || got != 510 {
		t.Fatalf("08:30 → %d, %v (ожидали 510)", got, err)
	}
	if _, err := parseHM("abc"); err == nil {
		t.Fatal("мусор должен давать ошибку")
	}
}

func TestWindowGrid(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatalf("tz: %v", err)
	}
	at := func(h, m int) time.Time { return time.Date(2026, 11, 16, h, m, 30, 0, loc) }
	day := newDayRange(480, 1200) // 08:00–20:00

	// 10:03 → окно 10:00–10:05 (округление вниз — «в каком окне момент»)
	if start, end, open := windowAt(at(10, 3), loc, day, 5); !open ||
		!start.Equal(time.Date(2026, 11, 16, 10, 0, 0, 0, loc)) || !end.Equal(start.Add(5*time.Minute)) {
		t.Fatalf("10:03 → %v %v %v", start, end, open)
	}
	// вне рабочих часов — не окно
	if _, _, open := windowAt(at(21, 0), loc, day, 5); open {
		t.Fatal("21:00 должно быть закрыто")
	}
	// до открытия — закрыто: «первое окно дня» назначает только
	// nextOpening (и дальше работает лимит maxLead)
	if _, _, open := windowAt(at(6, 0), loc, day, 5); open {
		t.Fatal("06:00 до открытия должно быть закрыто")
	}
	// nextOpening: до открытия — сегодня, после — завтра
	if got := nextOpening(at(6, 0), loc, day); !got.Equal(time.Date(2026, 11, 16, 8, 0, 0, 0, loc)) {
		t.Fatalf("nextOpening 06:00 → %v", got)
	}
	if got := nextOpening(at(21, 0), loc, day); !got.Equal(time.Date(2026, 11, 17, 8, 0, 0, 0, loc)) {
		t.Fatalf("nextOpening 21:00 → %v", got)
	}
}

func TestWindowGridOvernight(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Moscow")
	at := func(h, m int) time.Time { return time.Date(2026, 11, 16, h, m, 30, 0, loc) }
	night := newDayRange(6*60, 1*60) // 06:00–01:00 — 19 часов через полночь

	// 00:30 — ещё вчерашняя смена: окно 00:30–00:35 сегодня, но сетка от 06:00.
	if start, end, open := windowAt(at(0, 30), loc, night, 5); !open ||
		!start.Equal(time.Date(2026, 11, 16, 0, 30, 0, 0, loc)) || !end.Equal(start.Add(5*time.Minute)) {
		t.Fatalf("00:30 в ночной смене → %v %v %v", start, end, open)
	}
	// 06:00 — первое окно смены
	if start, _, open := windowAt(at(6, 0), loc, night, 5); !open ||
		!start.Equal(time.Date(2026, 11, 16, 6, 0, 0, 0, loc)) {
		t.Fatalf("06:00 → %v %v", start, open)
	}
	// 23:58 — последнее окно перед полуночью
	if start, _, open := windowAt(at(23, 58), loc, night, 5); !open ||
		!start.Equal(time.Date(2026, 11, 16, 23, 55, 0, 0, loc)) {
		t.Fatalf("23:58 → %v %v", start, open)
	}
	// 03:00 — между закрытием в 01:00 и открытием в 06:00
	if _, _, open := windowAt(at(3, 0), loc, night, 5); open {
		t.Fatal("03:00 должно быть закрыто")
	}
	if got := nextOpening(at(3, 0), loc, night); !got.Equal(time.Date(2026, 11, 16, 6, 0, 0, 0, loc)) {
		t.Fatalf("nextOpening 03:00 → %v", got)
	}
	// «Круглосуточно» (начало = конец): открыто в любое время, сетка от начала.
	allDay := newDayRange(8*60, 8*60)
	for _, h := range []int{0, 3, 9, 23} {
		if _, _, open := windowAt(at(h, 15), loc, allDay, 5); !open {
			t.Fatalf("%02d:15 круглосуточно должно быть открыто", h)
		}
	}
	if start, _, _ := windowAt(at(3, 2), loc, allDay, 5); !start.Equal(time.Date(2026, 11, 16, 3, 0, 0, 0, loc)) {
		t.Fatalf("сетка круглосуточной точки → %v", start)
	}
}

func TestOpenState(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Moscow")
	cfg := defaultCapacity() // 08:00–20:00
	if open, _, _ := openState(cfg, "Europe/Moscow", time.Date(2026, 11, 16, 9, 0, 0, 0, loc)); !open {
		t.Fatal("09:00 должно быть открыто")
	}
	if open, opensAt, _ := openState(cfg, "Europe/Moscow", time.Date(2026, 11, 16, 7, 0, 0, 0, loc)); open || opensAt != "08:00" {
		t.Fatalf("07:00: %v %q", open, opensAt)
	}

	// Перерыв 13:00–13:30: гость видит «перерыв до 13:30», а не «закрыто».
	cfg.Breaks = []Break{{Start: "13:00", End: "13:30"}}
	if open, opensAt, onBreak := openState(cfg, "Europe/Moscow", time.Date(2026, 11, 16, 13, 10, 0, 0, loc)); open || !onBreak || opensAt != "13:30" {
		t.Fatalf("13:10 на перерыве: open=%v break=%v opens=%q", open, onBreak, opensAt)
	}
	if open, _, _ := openState(cfg, "Europe/Moscow", time.Date(2026, 11, 16, 13, 40, 0, 0, loc)); !open {
		t.Fatal("13:40 после перерыва должно быть открыто")
	}
}

func TestOpenStateOvernight(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Moscow")
	at := func(h, m int) time.Time { return time.Date(2026, 11, 16, h, m, 0, 0, loc) }
	cfg := CapacityConfig{WindowMinutes: 5, MaxOrdersPerWindow: 3, WorkStart: "06:00", WorkEnd: "01:00", AcceptETAMinutes: defaultAcceptETA}

	for _, hm := range [][2]int{{6, 0}, {12, 0}, {23, 59}, {0, 0}, {0, 30}} {
		if open, _, _ := openState(cfg, "Europe/Moscow", at(hm[0], hm[1])); !open {
			t.Fatalf("%02d:%02d в смене 06:00–01:00 должно быть открыто", hm[0], hm[1])
		}
	}
	for _, hm := range [][2]int{{1, 0}, {3, 0}, {5, 59}} {
		if open, opensAt, _ := openState(cfg, "Europe/Moscow", at(hm[0], hm[1])); open || opensAt != "06:00" {
			t.Fatalf("%02d:%02d должно быть закрыто до 06:00: open=%v opens=%q", hm[0], hm[1], open, opensAt)
		}
	}

	// Начало = конец — круглосуточно.
	cfg.WorkStart, cfg.WorkEnd = "08:00", "08:00"
	for _, hm := range [][2]int{{0, 15}, {3, 0}, {8, 0}, {23, 0}} {
		if open, _, _ := openState(cfg, "Europe/Moscow", at(hm[0], hm[1])); !open {
			t.Fatalf("круглосуточно: %02d:%02d должно быть открыто", hm[0], hm[1])
		}
	}
}

func TestBreakOverlapEnd(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Moscow")
	day := func(h, m int) time.Time { return time.Date(2026, 11, 16, h, m, 0, 0, loc) }
	breaks := []breakRange{{start: 13 * 60, end: 13*60 + 30}}
	if got, ok := breakOverlapEnd(day(13, 0), day(13, 5), loc, breaks); !ok || !got.Equal(day(13, 30)) {
		t.Fatalf("окно внутри перерыва: %v %v", got, ok)
	}
	if _, ok := breakOverlapEnd(day(14, 0), day(14, 5), loc, breaks); ok {
		t.Fatal("окно после перерыва не пересекается")
	}

	// Перерыв через полночь 23:00–02:00: окно в 01:00 ждёт до 02:00, а окно
	// вечером в 22:00 не пересекается.
	nightBreak := []breakRange{{start: 23 * 60, end: 2 * 60}}
	if got, ok := breakOverlapEnd(day(1, 0), day(1, 5), loc, nightBreak); !ok || !got.Equal(day(2, 0)) {
		t.Fatalf("ночной перерыв в 01:00: %v %v", got, ok)
	}
	if got, ok := breakOverlapEnd(day(23, 10), day(23, 15), loc, nightBreak); !ok ||
		!got.Equal(time.Date(2026, 11, 17, 2, 0, 0, 0, loc)) {
		t.Fatalf("ночной перерыв в 23:10: %v %v", got, ok)
	}
	if _, ok := breakOverlapEnd(day(22, 0), day(22, 5), loc, nightBreak); ok {
		t.Fatal("22:00 до перерыва не пересекается")
	}
}
