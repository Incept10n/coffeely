package httpx

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// LogRequests — middleware журналирования каждого входящего запроса в stdout.
// Пишет одну строку JSON на запрос (slog): метод, путь, статус, размер ответа,
// длительность, клиент, user-agent и X-Request-Id (генерируется, если не пришёл).
// WebSocket-запросы логируются при закрытии соединения — с полной длительностью.
func LogRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		requestID := r.Header.Get("X-Request-Id")
		if requestID == "" {
			requestID = newRequestID()
		}
		w.Header().Set("X-Request-Id", requestID)

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"bytes", rec.bytes,
			"duration_ms", float64(time.Since(start).Microseconds()) / 1000,
			"remote", clientIP(r),
			"request_id", requestID,
		}
		if q := r.URL.RawQuery; q != "" {
			attrs = append(attrs, "query", redactQuery(q))
		}
		if ua := r.UserAgent(); ua != "" {
			attrs = append(attrs, "user_agent", ua)
		}

		level := slog.LevelInfo
		switch {
		case rec.status >= 500:
			level = slog.LevelError
		case rec.status >= 400:
			level = slog.LevelWarn
		}
		slog.Log(r.Context(), level, "http request", attrs...)
	})
}

// sensitiveQuery — параметры, которые не должны попадать в логи: одноразовые
// OAuth-коды, state и токены. Имя параметра остаётся (удобно для отладки),
// значение маскируется.
var sensitiveQuery = map[string]bool{
	"code":         true,
	"state":        true,
	"token":        true,
	"oauth_token":  true,
	"access_token": true,
}

func redactQuery(raw string) string {
	parts := strings.Split(raw, "&")
	for i, part := range parts {
		key, _, hasValue := strings.Cut(part, "=")
		if hasValue && sensitiveQuery[strings.ToLower(key)] {
			parts[i] = key + "=***"
		}
	}
	return strings.Join(parts, "&")
}

// statusRecorder запоминает статус и число байт ответа.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	bytes   int
	written bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.written {
		r.status = code
		r.written = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.written = true
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// Unwrap — чтобы http.ResponseController (Flush/Hijack/SetReadDeadline) видел
// исходный ResponseWriter: на этом держится WebSocket-хендшейк.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// Flush пробрасывает Flush исходному writer'у (стриминг ответов).
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack пробрасывает Hijack (WebSocket-апгрейд).
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := r.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}

// ClientIP — IP клиента за прокси (X-Forwarded-For / X-Real-IP / RemoteAddr).
// Экспортирован: его используют лимитеры доступа (админка).
func ClientIP(r *http.Request) string { return clientIP(r) }

func clientIP(r *http.Request) string {
	// За nginx: X-Real-IP выставляет сам прокси ($remote_addr) — ему доверяем
	// первым. X-Forwarded-For клиент может подделать (nginx только дописывает),
	// поэтому он — запасной вариант и берётся первым значением.
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return strings.TrimSpace(ip)
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
