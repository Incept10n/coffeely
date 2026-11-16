package points

import (
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
		{"переопределение лимита", `{"max_orders_per_window":1}`, CapacityConfig{5, 1, "08:00", "20:00"}},
		{"переопределение окна", `{"window_minutes":10}`, CapacityConfig{10, 3, "08:00", "20:00"}},
		{"нули игнорируются", `{"window_minutes":0,"max_orders_per_window":0}`, defaultCapacity()},
		{"часы работы", `{"work_start":"07:30","work_end":"22:00"}`, CapacityConfig{5, 3, "07:30", "22:00"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCapacity([]byte(tt.raw))
			if got != tt.want {
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
	if open, _ := openState(cfg, "Europe/Moscow", time.Date(2026, 11, 16, 9, 0, 0, 0, loc)); !open {
		t.Fatal("09:00 должно быть открыто")
	}
	if open, opensAt := openState(cfg, "Europe/Moscow", time.Date(2026, 11, 16, 7, 0, 0, 0, loc)); open || opensAt != "08:00" {
		t.Fatalf("07:00: %v %q", open, opensAt)
	}
}
