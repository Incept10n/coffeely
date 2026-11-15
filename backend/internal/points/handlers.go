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

var timeNow = time.Now
