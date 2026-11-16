package admin

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/Incept10n/coffeely/backend/internal/auth"
	"github.com/Incept10n/coffeely/backend/internal/httpx"
	"github.com/Incept10n/coffeely/backend/internal/securebox"
)

// Handlers — эндпоинты кабинета владельца.
type Handlers struct {
	store *Store
	box   *securebox.Box
}

func NewHandlers(store *Store, box *securebox.Box) *Handlers {
	return &Handlers{store: store, box: box}
}

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
	PriceMinor  int64  `json:"price_minor"`
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

type updatePointRequest struct {
	Title              *string `json:"title"`
	Address            *string `json:"address"`
	IsActive           *bool   `json:"is_active"`
	WindowMinutes      *int    `json:"window_minutes"`
	MaxOrdersPerWindow *int    `json:"max_orders_per_window"`
	WorkStart          *string `json:"work_start"`
	WorkEnd            *string `json:"work_end"`
}

// PATCH /admin/points/{id} — изменить информацию о точке.
func (h *Handlers) UpdatePoint(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.FromContext(r.Context())
	if !ok {
		httpx.WriteError(w, httpx.ErrUnauthorized)
		return
	}
	req, err := httpx.ReadJSON[updatePointRequest](r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	patch := PointPatch{
		IsActive:           req.IsActive,
		WindowMinutes:      req.WindowMinutes,
		MaxOrdersPerWindow: req.MaxOrdersPerWindow,
	}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			httpx.WriteError(w, httpx.ErrBadRequest)
			return
		}
		patch.Title = &title
	}
	if req.Address != nil {
		address := strings.TrimSpace(*req.Address)
		patch.Address = &address
	}
	if req.WorkStart != nil {
		if !hmRe.MatchString(*req.WorkStart) {
			httpx.WriteError(w, ErrBadCapacity)
			return
		}
		patch.WorkStart = req.WorkStart
	}
	if req.WorkEnd != nil {
		if !hmRe.MatchString(*req.WorkEnd) {
			httpx.WriteError(w, ErrBadCapacity)
			return
		}
		patch.WorkEnd = req.WorkEnd
	}
	if req.WindowMinutes != nil && (*req.WindowMinutes <= 0 || *req.WindowMinutes > 60) {
		httpx.WriteError(w, ErrBadCapacity)
		return
	}
	if req.MaxOrdersPerWindow != nil && (*req.MaxOrdersPerWindow <= 0 || *req.MaxOrdersPerWindow > 100) {
		httpx.WriteError(w, ErrBadCapacity)
		return
	}

	point, err := h.store.UpdatePoint(r.Context(), session.UserID, r.PathValue("id"), patch)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, point)
}

// GET /admin/points/{id}/menu — меню точки, включая скрытые позиции.
func (h *Handlers) ListMenu(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.FromContext(r.Context())
	if !ok {
		httpx.WriteError(w, httpx.ErrUnauthorized)
		return
	}
	items, err := h.store.ListMenu(r.Context(), session.UserID, r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

// POST /admin/points/{id}/menu — добавить позицию в меню точки.
func (h *Handlers) CreateMenuItem(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.FromContext(r.Context())
	if !ok {
		httpx.WriteError(w, httpx.ErrUnauthorized)
		return
	}
	req, err := httpx.ReadJSON[menuItemReq](r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" || req.PriceMinor <= 0 || req.PrepSeconds <= 0 {
		httpx.WriteError(w, httpx.ErrBadRequest)
		return
	}
	item, err := h.store.CreateMenuItem(r.Context(), session.UserID, r.PathValue("id"),
		req.Title, req.PriceMinor, req.PrepSeconds)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, item)
}

type updateMenuItemRequest struct {
	Title       *string `json:"title"`
	PriceMinor  *int64  `json:"price_minor"`
	PrepSeconds *int    `json:"prep_seconds"`
	IsAvailable *bool   `json:"is_available"`
}

// PATCH /admin/menu/{id} — изменить информацию о товаре.
func (h *Handlers) UpdateMenuItem(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.FromContext(r.Context())
	if !ok {
		httpx.WriteError(w, httpx.ErrUnauthorized)
		return
	}
	req, err := httpx.ReadJSON[updateMenuItemRequest](r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	patch := MenuItemPatch{
		PriceMinor:  req.PriceMinor,
		PrepSeconds: req.PrepSeconds,
		IsAvailable: req.IsAvailable,
	}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			httpx.WriteError(w, httpx.ErrBadRequest)
			return
		}
		patch.Title = &title
	}
	if req.PriceMinor != nil && *req.PriceMinor <= 0 {
		httpx.WriteError(w, httpx.ErrBadRequest)
		return
	}
	if req.PrepSeconds != nil && *req.PrepSeconds <= 0 {
		httpx.WriteError(w, httpx.ErrBadRequest)
		return
	}
	item, err := h.store.UpdateMenuItem(r.Context(), session.UserID, r.PathValue("id"), patch)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, item)
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
	enc, err := h.box.Encrypt(req.Password)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	barista, err := h.store.CreateBarista(r.Context(), req.Email, hash, enc, req.PointID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	barista.Password = req.Password
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
	for i := range list {
		h.decryptPassword(&list[i])
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"baristas": list})
}

type updateBaristaRequest struct {
	Email    *string `json:"email"`
	Password *string `json:"password"`
	PointID  *string `json:"point_id"`
}

// PATCH /admin/baristas/{id} — редактирование рабочего аккаунта: email,
// пароль и точка. Владелец может только своих бариста.
func (h *Handlers) UpdateBarista(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.FromContext(r.Context())
	if !ok {
		httpx.WriteError(w, httpx.ErrUnauthorized)
		return
	}
	req, err := httpx.ReadJSON[updateBaristaRequest](r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	patch := BaristaPatch{}
	if req.Email != nil {
		email := strings.TrimSpace(strings.ToLower(*req.Email))
		if email == "" {
			httpx.WriteError(w, httpx.ErrBadRequest)
			return
		}
		patch.Email = &email
	}
	if req.Password != nil {
		if len(*req.Password) < 8 {
			httpx.WriteError(w, httpx.ErrBadRequest)
			return
		}
		hash, err := auth.HashPassword(*req.Password)
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		enc, err := h.box.Encrypt(*req.Password)
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		patch.PasswordHash, patch.PasswordEnc = &hash, enc
	}
	if req.PointID != nil {
		if *req.PointID == "" {
			httpx.WriteError(w, httpx.ErrBadRequest)
			return
		}
		owned, err := h.store.PointOwnedBy(r.Context(), session.UserID, *req.PointID)
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		if !owned {
			httpx.WriteError(w, ErrForeignPoint)
			return
		}
		patch.PointID = req.PointID
	}

	barista, err := h.store.UpdateBarista(r.Context(), session.UserID, r.PathValue("id"), patch)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	h.decryptPassword(&barista)
	if req.Password != nil {
		barista.Password = *req.Password
	}
	httpx.WriteJSON(w, http.StatusOK, barista)
}

// decryptPassword заполняет Password из обратимой копии; сбой расшифровки
// (старый ключ, битые данные) не ломает список — пароль просто пустой.
func (h *Handlers) decryptPassword(b *Barista) {
	if len(b.PasswordEnc) == 0 {
		return
	}
	if pw, err := h.box.Decrypt(b.PasswordEnc); err == nil {
		b.Password = pw
	}
}
