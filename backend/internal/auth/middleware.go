package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/Incept10n/coffeely/backend/internal/httpx"
)

// ctxUser — значение контекста с аутентифицированным пользователем.
type ctxUser struct{}

// Session — данные access-токена в контексте запроса.
type Session struct {
	UserID string
	Role   string
	Email  string
}

// FromContext возвращает сессию, если запрос прошёл через Authenticate.
func FromContext(ctx context.Context) (Session, bool) {
	s, ok := ctx.Value(ctxUser{}).(Session)
	return s, ok
}

// Authenticate требует Bearer access-токен; кладёт Session в контекст или 401.
func (h *Handlers) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if header == "" || !ok || token == "" {
			httpx.WriteError(w, httpx.ErrUnauthorized)
			return
		}
		claims, err := ParseAccessToken(h.cfg, token)
		if err != nil {
			httpx.WriteError(w, httpx.ErrUnauthorized)
			return
		}
		session := Session{UserID: claims.Subject, Role: claims.Role, Email: claims.Email}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxUser{}, session)))
	})
}

// RequireRole пропускает запрос, только если роль совпадает; иначе 403 (401, если не аутентифицирован).
func RequireRole(role string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := FromContext(r.Context())
		if !ok {
			httpx.WriteError(w, httpx.ErrUnauthorized)
			return
		}
		if session.Role != role {
			httpx.WriteError(w, httpx.ErrForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func randState() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("yandex state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
