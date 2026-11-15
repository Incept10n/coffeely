package orders

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/Incept10n/coffeely/backend/internal/auth"
	"github.com/Incept10n/coffeely/backend/internal/httpx"
)

// Hub — realtime v1: подписки по точке (позже Centrifugo).
type Hub struct {
	mu   sync.Mutex
	subs map[string]map[chan []byte]struct{}
}

func NewHub() *Hub { return &Hub{subs: make(map[string]map[chan []byte]struct{})} }

// Subscribe регистрирует подписчика точки; cancel отписывает.
func (h *Hub) Subscribe(pointID string) (<-chan []byte, func()) {
	ch := make(chan []byte, 32)
	h.mu.Lock()
	if h.subs[pointID] == nil {
		h.subs[pointID] = make(map[chan []byte]struct{})
	}
	h.subs[pointID][ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs[pointID], ch)
		if len(h.subs[pointID]) == 0 {
			delete(h.subs, pointID)
		}
		h.mu.Unlock()
		close(ch)
	}
}

// Publish рассылает событие всем подписчикам точки (не блокируясь).
func (h *Hub) Publish(pointID string, event any) {
	raw := jsonRaw(event)
	h.mu.Lock()
	subs := make([]chan []byte, 0, len(h.subs[pointID]))
	for ch := range h.subs[pointID] {
		subs = append(subs, ch)
	}
	h.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- raw:
		default: // подписчик не успевает — пропускаем (v1)
		}
	}
}

// Handlers — HTTP-хендлеры заказов (клиент) и KDS (бариста).
type Handlers struct {
	store *Store
	hub   *Hub
}

func NewHandlers(store *Store, hub *Hub) *Handlers {
	return &Handlers{store: store, hub: hub}
}

// --- клиент (v1; движок ёмкости и отмена — M4) ---

type createOrderRequest struct {
	PointID string `json:"point_id"`
	// RFC3339, например "2026-11-15T10:00:00+03:00"
	PickupSlotStart time.Time `json:"pickup_slot_start"`
	PickupSlotEnd   time.Time `json:"pickup_slot_end"`
	Items           []struct {
		MenuItemID string `json:"menu_item_id"`
		Qty        int    `json:"qty"`
	} `json:"items"`
}

// POST /orders — создание заказа клиентом.
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.FromContext(r.Context())
	if !ok || session.Role != "client" && session.Role != "barista" {
		httpx.WriteError(w, httpx.ErrForbidden)
		return
	}
	req, err := httpx.ReadJSON[createOrderRequest](r)
	if err != nil || req.PointID == "" || len(req.Items) == 0 {
		httpx.WriteError(w, httpx.ErrBadRequest)
		return
	}
	items := make([]Item, len(req.Items))
	for i, it := range req.Items {
		items[i] = Item{MenuItemID: it.MenuItemID, Qty: it.Qty}
	}
	order, err := h.store.Create(r.Context(), req.PointID, session.UserID, req.PickupSlotStart, req.PickupSlotEnd, items)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	h.hub.Publish(order.PointID, json.RawMessage(jsonRaw(map[string]any{"type": "order_created", "order": order})))
	httpx.WriteJSON(w, http.StatusCreated, order)
}

// GET /orders/{id} — статус заказа: владелец-клиент или бариста точки.
// Гость смотрит статус поллингом (realtime для клиента — M4/M5).
func (h *Handlers) ByID(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.FromContext(r.Context())
	if !ok {
		httpx.WriteError(w, httpx.ErrUnauthorized)
		return
	}
	order, err := h.store.ByID(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if order.ClientID != session.UserID && session.Role != "barista" && session.Role != "owner" {
		httpx.WriteError(w, ErrForbiddenOrder)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, order)
}

// --- бариста (KDS) ---

// QueueResponse — очередь с вычисленной срочностью (ядро AIK-11).
type QueueEntry struct {
	Order
	// until_deadline в секундах; отрицательный — промах
	UntilDeadline int64  `json:"until_deadline_seconds"`
	Urgency       string `json:"urgency"` // "later" | "soon" | "now" | "overdue"
}

// GET /barista/queue — активные заказы точки баристы.
func (h *Handlers) Queue(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.FromContext(r.Context())
	if !ok || session.Role != "barista" {
		httpx.WriteError(w, httpx.ErrForbidden)
		return
	}
	pointID, err := h.store.PointOfBarista(r.Context(), session.UserID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	orders, err := h.store.QueueOfPoint(r.Context(), pointID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	now := time.Now()
	entries := make([]QueueEntry, len(orders))
	for i, o := range orders {
		d := o.Deadline.Sub(now)
		e := QueueEntry{Order: o, UntilDeadline: int64(d / time.Second), Urgency: urgency(d)}
		entries[i] = e
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"point_id": pointID, "orders": entries})
}

// urgency — «№412 — к 8:41, делать сейчас».
func urgency(d time.Duration) string {
	switch {
	case d < 0:
		return "overdue"
	case d < 2*time.Minute:
		return "now"
	case d < 5*time.Minute:
		return "soon"
	default:
		return "later"
	}
}

type statusRequest struct {
	Status string `json:"status"`
}

// POST /barista/orders/{id}/status — смена статуса с валидацией перехода.
func (h *Handlers) SetStatus(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.FromContext(r.Context())
	if !ok || session.Role != "barista" {
		httpx.WriteError(w, httpx.ErrForbidden)
		return
	}
	pointID, err := h.store.PointOfBarista(r.Context(), session.UserID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	req, err := httpx.ReadJSON[statusRequest](r)
	if err != nil {
		httpx.WriteError(w, httpx.ErrBadRequest)
		return
	}

	order, err := h.store.ByID(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if order.PointID != pointID {
		httpx.WriteError(w, ErrForbiddenOrder)
		return
	}

	updated, err := h.store.Transition(r.Context(), order.ID, session.UserID, req.Status, time.Now())
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	h.hub.Publish(pointID, json.RawMessage(jsonRaw(map[string]any{"type": "order_status", "order": updated})))
	httpx.WriteJSON(w, http.StatusOK, updated)
}

// GET /barista/metrics?day=2026-11-15 — % дедлайнов, соблюдённых вовремя.
func (h *Handlers) Metrics(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.FromContext(r.Context())
	if !ok || session.Role != "barista" {
		httpx.WriteError(w, httpx.ErrForbidden)
		return
	}
	pointID, err := h.store.PointOfBarista(r.Context(), session.UserID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	day := time.Now()
	if q := r.URL.Query().Get("day"); q != "" {
		day, err = time.Parse("2006-01-02", q)
		if err != nil {
			httpx.WriteError(w, httpx.ErrBadRequest)
			return
		}
	}
	onTime, missed, err := h.store.Metrics(r.Context(), pointID, day)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	total := onTime + missed
	pct := 0.0
	if total > 0 {
		pct = float64(onTime) / float64(total) * 100
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"day": day.Format("2006-01-02"), "on_time": onTime,
		"missed": missed, "on_time_pct": strconv.FormatFloat(pct, 'f', 1, 64),
	})
}

// GET /ws/points/{id}?token=JWT — realtime-канал точки для баристы.
// Токен в query, т.к. браузерные WS не умеют заголовки.
func (h *Handlers) PointWS(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.FromContext(r.Context())
	if !ok || session.Role != "barista" {
		httpx.WriteError(w, httpx.ErrForbidden)
		return
	}
	pointID := r.PathValue("id")
	ownPoint, err := h.store.PointOfBarista(r.Context(), session.UserID)
	if err != nil || ownPoint != pointID {
		httpx.WriteError(w, ErrForbiddenOrder)
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusInternalError, "server closing")

	events, cancel := h.hub.Subscribe(pointID)
	defer cancel()

	// reader: ловим закрытие клиентом
	wsCtx, wsCancel := context.WithCancel(r.Context())
	defer wsCancel()
	go func() {
		defer wsCancel()
		for {
			if _, _, err := conn.Read(wsCtx); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-wsCtx.Done():
			conn.Close(websocket.StatusNormalClosure, "")
			return
		case msg, open := <-events:
			if !open {
				conn.Close(websocket.StatusNormalClosure, "")
				return
			}
			writeCtx, writeCancel := context.WithTimeout(wsCtx, 5*time.Second)
			err := conn.Write(writeCtx, websocket.MessageText, msg)
			writeCancel()
			if err != nil {
				slog.Debug("ws write failed", "err", err)
				return
			}
		}
	}
}
