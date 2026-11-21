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

// GET /points — активные точки с текущей загрузкой: сколько ждать и открыто ли.
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.List(r.Context(), timeNow())
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if list == nil {
		list = []Point{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"points": list})
}

// GET /points/{id}/menu — меню точки и доступные добавки.
func (h *Handlers) Menu(w http.ResponseWriter, r *http.Request) {
	pointID := r.PathValue("id")
	menu, err := h.store.Menu(r.Context(), pointID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	addons, err := h.store.Addons(r.Context(), pointID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if menu == nil {
		menu = []MenuItem{}
	}
	if addons == nil {
		addons = []Addon{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"menu": menu, "addons": addons})
}

var timeNow = time.Now
