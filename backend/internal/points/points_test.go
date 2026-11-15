package points

import "testing"

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
