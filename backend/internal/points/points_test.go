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

	// 10:03 → окно 10:00–10:05 (округление вниз — «в каком окне момент»)
	if start, end, open := windowAt(at(10, 3), loc, 480, 1200, 5); !open ||
		!start.Equal(time.Date(2026, 11, 16, 10, 0, 0, 0, loc)) || !end.Equal(start.Add(5*time.Minute)) {
		t.Fatalf("10:03 → %v %v %v", start, end, open)
	}
	// вне рабочих часов — не окно
	if _, _, open := windowAt(at(21, 0), loc, 480, 1200, 5); open {
		t.Fatal("21:00 должно быть закрыто")
	}
	// до открытия — закрыто: «первое окно дня» назначает только
	// nextOpening (и дальше работает лимит maxLead)
	if _, _, open := windowAt(at(6, 0), loc, 480, 1200, 5); open {
		t.Fatal("06:00 до открытия должно быть закрыто")
	}
	// nextOpening: до открытия — сегодня, после — завтра
	if got := nextOpening(at(6, 0), loc, 480); !got.Equal(time.Date(2026, 11, 16, 8, 0, 0, 0, loc)) {
		t.Fatalf("nextOpening 06:00 → %v", got)
	}
	if got := nextOpening(at(21, 0), loc, 480); !got.Equal(time.Date(2026, 11, 17, 8, 0, 0, 0, loc)) {
		t.Fatalf("nextOpening 21:00 → %v", got)
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
}
