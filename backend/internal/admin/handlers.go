package admin

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/Incept10n/coffeely/backend/internal/auth"
	"github.com/Incept10n/coffeely/backend/internal/httpx"
)

// Handlers — эндпоинты кабинета владельца.
type Handlers struct {
	store *Store
}

func NewHandlers(store *Store) *Handlers { return &Handlers{store: store} }

// RequireOwner — authenticate + роль owner. Админка существует только на сайте:
// клиент и бариста сюда не допускаются (403), неаутентифицированные — 401.
func (h *Handlers) RequireOwner(authenticate func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, ok := auth.FromContext(r.Context())
			if !ok {
				httpx.WriteError(w, httpx.ErrUnauthorized)
				return
			}
			if session.Role != "owner" {
				httpx.WriteError(w, httpx.ErrForbidden)
				return
			}
			next.ServeHTTP(w, r)
		}))
	}
}

var hmRe = regexp.MustCompile(`^([01]?\d|2[0-3]):[0-5]\d$`)

type menuItemReq struct {
	Title       string `json:"title"`
	PriceMinor  int    `json:"price_minor"`
	PrepSeconds int    `json:"prep_seconds"`
}

type createPointRequest struct {
	Title         string        `json:"title"`
	Address       string        `json:"address"`
	WindowMinutes int           `json:"window_minutes"`
	MaxPerWindow  int           `json:"max_orders_per_window"`
	WorkStart     string        `json:"work_start"` // HH:MM
	WorkEnd       string        `json:"work_end"`   // HH:MM
	Menu          []menuItemReq `json:"menu"`
}

// POST /admin/points — создать точку (+ необязательное меню).
func (h *Handlers) CreatePoint(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.FromContext(r.Context())
	if !ok {
		httpx.WriteError(w, httpx.ErrUnauthorized)
		return
	}
	req, err := httpx.ReadJSON[createPointRequest](r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" || req.WindowMinutes <= 0 || req.WindowMinutes > 60 ||
		req.MaxPerWindow <= 0 || req.MaxPerWindow > 100 ||
		!hmRe.MatchString(req.WorkStart) || !hmRe.MatchString(req.WorkEnd) {
		httpx.WriteError(w, ErrBadCapacity)
		return
	}
	cap, err := json.Marshal(map[string]any{
		"window_minutes":        req.WindowMinutes,
		"max_orders_per_window": req.MaxPerWindow,
		"work_start":            req.WorkStart,
		"work_end":              req.WorkEnd,
	})
	if err != nil {
		httpx.WriteError(w, ErrBadCapacity)
		return
	}
	menu := make([]MenuItem, 0, len(req.Menu))
	for _, mi := range req.Menu {
		if strings.TrimSpace(mi.Title) == "" || mi.PriceMinor <= 0 || mi.PrepSeconds <= 0 {
			httpx.WriteError(w, httpx.ErrBadRequest)
			return
		}
		menu = append(menu, MenuItem{Title: strings.TrimSpace(mi.Title), PriceMinor: mi.PriceMinor, PrepSeconds: mi.PrepSeconds})
	}

	point, err := h.store.CreatePoint(r.Context(), session.UserID, req.Title, req.Address, cap, menu)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, point)
}

// GET /admin/points — точки владельца.
func (h *Handlers) ListPoints(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.FromContext(r.Context())
	if !ok {
		httpx.WriteError(w, httpx.ErrUnauthorized)
		return
	}
	points, err := h.store.ListPoints(r.Context(), session.UserID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"points": points})
}

type createBaristaRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	PointID  string `json:"point_id"`
}

// POST /admin/baristas — создать рабочий аккаунт баристы на точку владельца.
func (h *Handlers) CreateBarista(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.FromContext(r.Context())
	if !ok {
		httpx.WriteError(w, httpx.ErrUnauthorized)
		return
	}
	req, err := httpx.ReadJSON[createBaristaRequest](r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || len(req.Password) < 8 || req.PointID == "" {
		httpx.WriteError(w, httpx.ErrBadRequest)
		return
	}
	owned, err := h.store.PointOwnedBy(r.Context(), session.UserID, req.PointID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if !owned {
		httpx.WriteError(w, ErrForeignPoint)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	barista, err := h.store.CreateBarista(r.Context(), req.Email, hash, req.PointID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, barista)
}

// GET /admin/baristas — рабочие аккаунты всех точек владельца.
func (h *Handlers) ListBaristas(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.FromContext(r.Context())
	if !ok {
		httpx.WriteError(w, httpx.ErrUnauthorized)
		return
	}
	list, err := h.store.ListBaristas(r.Context(), session.UserID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"baristas": list})
}
