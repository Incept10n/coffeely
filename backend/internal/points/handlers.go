package points

import (
	"net/http"
	"time"

	"github.com/Incept10n/coffeely/backend/internal/httpx"
)

// Handlers — клиентские эндпоинты точек.
type Handlers struct {
	store *Store
}

func NewHandlers(store *Store) *Handlers { return &Handlers{store: store} }

// GET /points — список активных точек.
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.List(r.Context())
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if list == nil {
		list = []Point{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"points": list})
}

// GET /points/{id}/menu — меню точки.
func (h *Handlers) Menu(w http.ResponseWriter, r *http.Request) {
	menu, err := h.store.Menu(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if menu == nil {
		menu = []MenuItem{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"menu": menu})
}

// GET /points/{id}/windows?day=YYYY-MM-DD — окна выдачи со свободной ёмкостью.
func (h *Handlers) Windows(w http.ResponseWriter, r *http.Request) {
	day := timeNow()
	if q := r.URL.Query().Get("day"); q != "" {
		parsed, err := time.Parse("2006-01-02", q)
		if err != nil {
			httpx.WriteError(w, httpx.ErrBadRequest)
			return
		}
		day = parsed
	}
	windows, err := h.store.Windows(r.Context(), r.PathValue("id"), day)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if windows == nil {
		windows = []Window{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"windows": windows})
}

var timeNow = time.Now
