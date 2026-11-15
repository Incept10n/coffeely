// Package httpx — мелкие HTTP-помощники: JSON-ответы, ошибки, чтение тела.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Error — API-ошибка, которую возвращают хендлеры. Is() сравнивает по коду.
type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

func (e *Error) Is(target error) bool {
	other, ok := target.(*Error)
	return ok && other != nil && other.Code == e.Code
}

func NewError(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

var (
	ErrBadRequest   = NewError(http.StatusBadRequest, "bad_request", "неверный запрос")
	ErrUnauthorized = NewError(http.StatusUnauthorized, "unauthorized", "требуется аутентификация")
	ErrForbidden    = NewError(http.StatusForbidden, "forbidden", "недостаточно прав")
	ErrConflict     = NewError(http.StatusConflict, "conflict", "конфликт данных")
	ErrGone         = NewError(http.StatusGone, "gone", "ресурс недоступен")
	ErrInternal     = NewError(http.StatusInternalServerError, "internal", "внутренняя ошибка")
)

// WriteJSON пишет v как JSON с указанным статусом.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError пишет *Error; прочие ошибки — как 500.
func WriteError(w http.ResponseWriter, err error) {
	apiErr := &Error{}
	if errors.As(err, &apiErr) {
		WriteJSON(w, apiErr.Status, apiErr)
		return
	}
	WriteJSON(w, http.StatusInternalServerError, ErrInternal)
}

// ReadJSON разбирает тело запроса как JSON ограниченного размера.
func ReadJSON[T any](r *http.Request) (T, error) {
	var v T
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		return v, fmt.Errorf("%w: %s", ErrBadRequest, err)
	}
	return v, nil
}
