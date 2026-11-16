package auth

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/Incept10n/coffeely/backend/internal/config"
	"github.com/Incept10n/coffeely/backend/internal/httpx"
)

// Handlers — HTTP-хендлеры аутентификации.
type Handlers struct {
	svc    *Service
	yandex *Yandex
	cfg    config.Config
}

func NewHandlers(svc *Service, yandex *Yandex, cfg config.Config) *Handlers {
	return &Handlers{svc: svc, yandex: yandex, cfg: cfg}
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type authResponse struct {
	User   User      `json:"user"`
	Tokens TokenPair `json:"tokens"`
}

// registerResponse — регистрация может требовать подтверждения email:
// тогда tokens отсутствуют, а клиент показывает экран ввода кода.
type registerResponse struct {
	User                 User       `json:"user"`
	Tokens               *TokenPair `json:"tokens,omitempty"`
	VerificationRequired bool       `json:"verification_required"`
	EmailSent            bool       `json:"email_sent"`
}

type verifyEmailRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type resendVerificationRequest struct {
	Email string `json:"email"`
}

// POST /auth/register
func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.ReadJSON[registerRequest](r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	out, err := h.svc.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, registerResponse{
		User:                 out.User,
		Tokens:               out.Tokens,
		VerificationRequired: out.VerificationRequired,
		EmailSent:            out.EmailSent,
	})
}

// POST /auth/verify-email — код из письма → подтверждение и токены.
func (h *Handlers) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.ReadJSON[verifyEmailRequest](r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	user, pair, err := h.svc.VerifyEmail(r.Context(), req.Email, req.Code)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, authResponse{User: user, Tokens: pair})
}

// POST /auth/resend-verification — новое письмо с кодом (не чаще раза в минуту).
func (h *Handlers) ResendVerification(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.ReadJSON[resendVerificationRequest](r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	sent, retryAfter, err := h.svc.ResendVerification(r.Context(), req.Email)
	if err != nil {
		if retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
		}
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"email_sent":          sent,
		"retry_after_seconds": int(retryAfter.Seconds()),
	})
}

// POST /auth/login
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.ReadJSON[loginRequest](r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	user, pair, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, authResponse{User: user, Tokens: pair})
}

// POST /owners/register — регистрация владельца кофейни на сайте (кабинет).
func (h *Handlers) OwnersRegister(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.ReadJSON[registerRequest](r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	out, err := h.svc.RegisterOwner(r.Context(), req.Email, req.Password)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, registerResponse{
		User:   out.User,
		Tokens: out.Tokens,
	})
}

// POST /owners/login — вход владельца в кабинет на сайте.
func (h *Handlers) OwnersLogin(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.ReadJSON[loginRequest](r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	user, pair, err := h.svc.LoginOwner(r.Context(), req.Email, req.Password)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, authResponse{User: user, Tokens: pair})
}

// POST /auth/refresh
func (h *Handlers) Refresh(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.ReadJSON[refreshRequest](r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	user, pair, err := h.svc.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, authResponse{User: user, Tokens: pair})
}

// POST /auth/logout
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.ReadJSON[logoutRequest](r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if err := h.svc.Logout(r.Context(), req.RefreshToken); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /auth/me
func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	session, ok := FromContext(r.Context())
	if !ok {
		httpx.WriteError(w, httpx.ErrUnauthorized)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, User{ID: session.UserID, Email: session.Email, Role: session.Role})
}

// GET /auth/yandex/start → редирект на Yandex ID.
func (h *Handlers) YandexStart(w http.ResponseWriter, r *http.Request) {
	startURL, err := h.yandex.StartURL()
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	http.Redirect(w, r, startURL, http.StatusFound)
}

// GET /auth/yandex/callback?code=..&state=.. → вход/связывание по email, выдача токенов.
func (h *Handlers) YandexCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		httpx.WriteError(w, httpx.ErrBadRequest)
		return
	}

	yandexID, email, err := h.yandex.Callback(r.Context(), code, state)
	if err != nil {
		httpx.WriteError(w, httpx.NewError(http.StatusUnauthorized, "yandex", err.Error()))
		return
	}

	user, pair, err := h.svc.LoginYandex(r.Context(), yandexID, email)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	// Вариант для API-клиента без браузера: JSON вместо редиректа в приложение.
	if r.URL.Query().Get("format") == "json" {
		httpx.WriteJSON(w, http.StatusOK, authResponse{User: user, Tokens: pair})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(authResponse{User: user, Tokens: pair})
}

// yandexTokenRequest — токен, который LoginSDK выдал приложению на устройстве.
type yandexTokenRequest struct {
	Token string `json:"token"`
}

// POST /auth/yandex/token — вход по токену из LoginSDK (мобильное приложение).
// Токен проверяется через login.yandex.ru/info: подпись гарантирует Яндекс,
// client_id в ответе — что токен выдан именно нашему приложению.
func (h *Handlers) YandexToken(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.YandexEnabled() {
		httpx.WriteError(w, httpx.NewError(http.StatusServiceUnavailable,
			"yandex_disabled", "вход через Яндекс не настроен"))
		return
	}
	req, err := httpx.ReadJSON[yandexTokenRequest](r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if strings.TrimSpace(req.Token) == "" {
		httpx.WriteError(w, httpx.NewError(http.StatusBadRequest,
			"yandex_token", "не передан токен Яндекса"))
		return
	}

	yandexID, email, err := h.yandex.Profile(r.Context(), req.Token)
	if err != nil {
		// Детали (в т.ч. ответ Яндекса) наружу не отдаём: они попадут в логи клиента.
		httpx.WriteError(w, httpx.NewError(http.StatusUnauthorized,
			"yandex_token", "не удалось подтвердить вход через Яндекс"))
		return
	}
	user, pair, err := h.svc.LoginYandex(r.Context(), yandexID, email)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, authResponse{User: user, Tokens: pair})
}

// GET /auth/providers — какие способы входа настроены на сервере.
// Приложение по этому ответу показывает или прячет кнопку «Войти с Яндексом».
func (h *Handlers) Providers(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{
		"yandex": h.cfg.YandexEnabled(),
	})
}
