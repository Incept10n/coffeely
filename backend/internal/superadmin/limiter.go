// Rate limiter для секретного входа: одна и та же строка на всех — значит,
// перебор надо ограничивать, иначе её подберут.
package superadmin

import (
	"sync"
	"time"
)

// Limiter — счётчик неудачных попыток по ключу (обычно IP).
type Limiter struct {
	mu      sync.Mutex
	entries map[string]*entry
	max     int
	window  time.Duration
	now     func() time.Time
}

type entry struct {
	failures int
	reset    time.Time
}

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{
		entries: make(map[string]*entry),
		max:     max,
		window:  window,
		now:     time.Now,
	}
}

// Allowed — можно ли ещё пробовать (не увеличивает счётчик).
func (l *Limiter) Allowed(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok {
		return true
	}
	if l.now().After(e.reset) {
		delete(l.entries, key)
		return true
	}
	return e.failures < l.max
}

// Fail фиксирует неудачную попытку.
func (l *Limiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.entries) > 4096 { // редкий случай мусорных ключей
		l.sweepLocked()
	}
	e, ok := l.entries[key]
	if !ok || l.now().After(e.reset) {
		l.entries[key] = &entry{failures: 1, reset: l.now().Add(l.window)}
		return
	}
	e.failures++
}

// Reset — успешный вход: счётчик сбрасывается.
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

func (l *Limiter) sweepLocked() {
	now := l.now()
	for k, e := range l.entries {
		if now.After(e.reset) {
			delete(l.entries, k)
		}
	}
}
