package orders

import (
	"testing"
	"time"
)

// Порог срочности — часть продуктового ядра («делать сейчас», AIK-11).
func TestUrgency(t *testing.T) {
	tests := []struct {
		left time.Duration
		want string
	}{
		{-1 * time.Minute, "overdue"},
		{0, "now"},
		{90 * time.Second, "now"},
		{3 * time.Minute, "soon"},
		{4*time.Minute + 59*time.Second, "soon"},
		{10 * time.Minute, "later"},
	}
	for _, tt := range tests {
		if got := urgency(tt.left); got != tt.want {
			t.Fatalf("urgency(%v) = %q, ожидали %q", tt.left, got, tt.want)
		}
	}
}
