package barista

import (
	"net/http"
	"time"

	"github.com/Incept10n/coffeely/backend/internal/auth"
	"github.com/Incept10n/coffeely/backend/internal/httpx"
)

// Handlers — HTTP-хендлеры баристы.
type Handlers struct {
	store *Store
}

func NewHandlers(store *Store) *Handlers { return &Handlers{store: store} }

type claimInviteRequest struct {
	Code string `json:"code"`
}

type claimInviteResponse struct {
	PointID string `json:"point_id"`
}

// POST /barista/claim-invite — любой аутентифицированный пользователь
// (кроме owner) становится баристой точки по одноразовому коду.
func (h *Handlers) ClaimInvite(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.FromContext(r.Context())
	if !ok {
		httpx.WriteError(w, httpx.ErrUnauthorized)
		return
	}
	if session.Role == "owner" {
		httpx.WriteError(w, httpx.ErrForbidden)
		return
	}
	req, err := httpx.ReadJSON[claimInviteRequest](r)
	if err != nil || req.Code == "" {
		httpx.WriteError(w, httpx.ErrBadRequest)
		return
	}
	pointID, err := h.store.Claim(r.Context(), session.UserID, req.Code)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, claimInviteResponse{PointID: pointID})
}

type createInviteRequest struct {
	PointID string `json:"point_id"`
	TTLHours int   `json:"ttl_hours,omitempty"` // дефолт 168 (7 дней)
}

type createInviteResponse struct {
	Code      string `json:"code"`
	ExpiresAt string `json:"expires_at"`
}

// POST /admin/invite-codes — только owner: выпуск кода приглашения.
func (h *Handlers) CreateInvite(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.FromContext(r.Context())
	if !ok || session.Role != "owner" {
		httpx.WriteError(w, httpx.ErrForbidden)
		return
	}
	req, err := httpx.ReadJSON[createInviteRequest](r)
	if err != nil || req.PointID == "" {
		httpx.WriteError(w, httpx.ErrBadRequest)
		return
	}
	ttl := 168 * time.Hour
	if req.TTLHours > 0 {
		ttl = time.Duration(req.TTLHours) * time.Hour
	}
	ic, err := h.store.CreateInviteCode(r.Context(), req.PointID, session.UserID, ttl)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, createInviteResponse{
		Code:      ic.Code,
		ExpiresAt: ic.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

// RegisterRoutes подключает маршруты баристы (authenticate оборачивается снаружи).
func (h *Handlers) RegisterRoutes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	mux.Handle("POST /barista/claim-invite", authenticate(http.HandlerFunc(h.ClaimInvite)))
	mux.Handle("POST /admin/invite-codes", authenticate(http.HandlerFunc(h.CreateInvite)))
}
