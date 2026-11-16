package superadmin

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Incept10n/coffeely/backend/internal/httpx"
)

// Handlers — вход по общему секрету и выдача аналитики.
type Handlers struct {
	store   *Store
	secret  string
	limiter *Limiter
}

func NewHandlers(store *Store, secret string) *Handlers {
	return &Handlers{
		store:   store,
		secret:  secret,
		limiter: NewLimiter(20, 15*time.Minute),
	}
}

var (
	errDisabled = httpx.NewError(http.StatusServiceUnavailable, "admin_disabled",
		"внутренняя админка отключена: не задан ADMIN_CRM_SECRET")
	errBadSecret = httpx.NewError(http.StatusUnauthorized, "bad_secret",
		"неверный секрет")
	errTooMany = httpx.NewError(http.StatusTooManyRequests, "too_many_attempts",
		"слишком много попыток, попробуйте позже")
)

// RequireSecret — доступ по заголовку X-Admin-Secret (не query: query попадает
// в логи nginx и нашего журнала).
func (h *Handlers) RequireSecret(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.secret == "" {
			httpx.WriteError(w, errDisabled)
			return
		}
		ip := httpx.ClientIP(r)
		if !h.limiter.Allowed(ip) {
			httpx.WriteError(w, errTooMany)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Admin-Secret")), []byte(h.secret)) != 1 {
			h.limiter.Fail(ip)
			httpx.WriteError(w, errBadSecret)
			return
		}
		h.limiter.Reset(ip)
		next.ServeHTTP(w, r)
	})
}

type loginRequest struct {
	Secret string `json:"secret"`
}

// POST /internal/admin/login — проверка строки доступа без загрузки аналитики.
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	if h.secret == "" {
		httpx.WriteError(w, errDisabled)
		return
	}
	req, err := httpx.ReadJSON[loginRequest](r)
	if err != nil || req.Secret == "" {
		httpx.WriteError(w, httpx.ErrBadRequest)
		return
	}
	ip := httpx.ClientIP(r)
	if !h.limiter.Allowed(ip) {
		httpx.WriteError(w, errTooMany)
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Secret), []byte(h.secret)) != 1 {
		h.limiter.Fail(ip)
		httpx.WriteError(w, errBadSecret)
		return
	}
	h.limiter.Reset(ip)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GET /internal/admin/analytics?days=30 — сводная аналитика платформы.
func (h *Handlers) Analytics(w http.ResponseWriter, r *http.Request) {
	days := 30
	if q := strings.TrimSpace(r.URL.Query().Get("days")); q != "" {
		n, err := strconv.Atoi(q)
		if err != nil || n < 1 || n > 365 {
			httpx.WriteError(w, httpx.ErrBadRequest)
			return
		}
		days = n
	}
	a, err := h.store.Analytics(r.Context(), days, time.Now())
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, a)
}
