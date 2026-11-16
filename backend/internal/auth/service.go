package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/mail"
	"strings"
	"time"

	"github.com/Incept10n/coffeely/backend/internal/config"
	"github.com/Incept10n/coffeely/backend/internal/httpx"
)

// CodeSender отправляет код подтверждения email. nil — подтверждение выключено
// (dev без почты): регистрация сразу выдаёт токены.
type CodeSender interface {
	SendVerificationCode(ctx context.Context, to, code string) error
}

// Service — бизнес-логика аутентификации: регистрация, вход, ротация refresh, Yandex.
type Service struct {
	store  *Store
	cfg    config.Config
	mailer CodeSender
}

func NewService(store *Store, cfg config.Config, mailer CodeSender) *Service {
	return &Service{store: store, cfg: cfg, mailer: mailer}
}

// RegisterOutcome — результат регистрации. Tokens пуст, если нужно подтвердить
// email: тогда клиент вводит код и получает токены из VerifyEmail.
type RegisterOutcome struct {
	User                 User
	Tokens               *TokenPair
	VerificationRequired bool
	EmailSent            bool
}

// Register создаёт клиента (role=client). При настроенной почте аккаунт
// неактивен до ввода кода из письма: /auth/verify-email выдаёт токены.
// Владелец регистрируется на сайте (RegisterOwner).
func (s *Service) Register(ctx context.Context, email, password string) (RegisterOutcome, error) {
	return s.registerAs(ctx, email, password, "client")
}

// RegisterOwner — регистрация владельца кофейни на сайте (личный кабинет).
// Email владельца считается подтверждённым: кабинет не должен зависеть от почты.
func (s *Service) RegisterOwner(ctx context.Context, email, password string) (RegisterOutcome, error) {
	return s.registerAs(ctx, email, password, "owner")
}

func (s *Service) registerAs(ctx context.Context, email, password, role string) (RegisterOutcome, error) {
	if _, err := mail.ParseAddress(email); err != nil {
		return RegisterOutcome{}, fmt.Errorf("неверный email: %w", ErrBadCredentials)
	}
	if len(password) < 8 {
		return RegisterOutcome{}, fmt.Errorf("пароль короче 8 символов: %w", ErrBadCredentials)
	}
	email = strings.ToLower(strings.TrimSpace(email))

	// Проверять email имеет смысл только для гостя приложения и только если
	// почтовый канал настроен. Владельцы и бариста — доверенные аккаунты.
	verify := role == "client" && s.mailer != nil

	hash, err := HashPassword(password)
	if err != nil {
		return RegisterOutcome{}, err
	}
	user, err := s.store.CreateUser(ctx, email, role, hash, "", !verify)
	if err != nil {
		return RegisterOutcome{}, err
	}

	if !verify {
		pair, err := s.issuePair(ctx, user)
		if err != nil {
			return RegisterOutcome{}, err
		}
		return RegisterOutcome{User: user, Tokens: &pair}, nil
	}

	sent := s.sendVerificationCode(ctx, user)
	return RegisterOutcome{
		User:                 user,
		VerificationRequired: true,
		EmailSent:            sent,
	}, nil
}

// VerifyEmail проверяет код из письма, подтверждает email и выдаёт токены.
// Повторный вызов для уже подтверждённого аккаунта просто логинит.
func (s *Service) VerifyEmail(ctx context.Context, email, code string) (User, TokenPair, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	code = strings.Join(strings.Fields(code), "")

	user, _, _, verified, err := s.store.UserByEmail(ctx, email)
	if err != nil {
		return User{}, TokenPair{}, err
	}
	if verified {
		pair, err := s.issuePair(ctx, user)
		return user, pair, err
	}

	active, err := s.store.ActiveVerificationCode(ctx, user.ID)
	if err != nil {
		return User{}, TokenPair{}, err
	}
	if active.Attempts >= s.cfg.VerificationMaxAttempts {
		return User{}, TokenPair{}, ErrTooManyAttempts
	}
	if time.Now().After(active.ExpiresAt) {
		return User{}, TokenPair{}, ErrCodeExpired
	}
	// Считаем попытку до сравнения: подбор не должен быть бесплатным.
	if err := s.store.IncrementCodeAttempts(ctx, active.ID); err != nil {
		return User{}, TokenPair{}, err
	}
	if !hmac.Equal([]byte(hashVerificationCode(s.cfg.JWTSecret, user.ID, code)), []byte(active.CodeHash)) {
		return User{}, TokenPair{}, ErrCodeNotFound
	}
	if err := s.store.MarkEmailVerified(ctx, user.ID, active.ID); err != nil {
		return User{}, TokenPair{}, err
	}
	pair, err := s.issuePair(ctx, user)
	return user, pair, err
}

// ResendVerification отправляет новый код, если прошло достаточно времени.
// Для неизвестного/подтверждённого email — (false, 0, nil): не раскрываем,
// зарегистрирован ли адрес.
func (s *Service) ResendVerification(ctx context.Context, email string) (bool, time.Duration, error) {
	if s.mailer == nil {
		return false, 0, nil
	}
	email = strings.ToLower(strings.TrimSpace(email))
	user, _, _, verified, err := s.store.UserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrBadCredentials) {
			return false, 0, nil
		}
		return false, 0, err
	}
	if verified || user.Role != "client" {
		return false, 0, nil
	}

	if last, ok, err := s.store.LastVerificationCodeAt(ctx, user.ID); err != nil {
		return false, 0, err
	} else if ok {
		if wait := s.cfg.VerificationResendPeriod - time.Since(last); wait > 0 {
			return false, wait, ErrResendTooSoon
		}
	}
	return s.sendVerificationCode(ctx, user), 0, nil
}

// sendVerificationCode создаёт код, сохраняет хеш и отправляет письмо.
// Ошибка почты не отменяет регистрацию: пользователь запросит код повторно.
func (s *Service) sendVerificationCode(ctx context.Context, user User) bool {
	if s.mailer == nil {
		return false
	}
	code, err := newVerificationCode()
	if err != nil {
		slog.Error("verification code generate", "user_id", user.ID, "err", err)
		return false
	}
	if _, err := s.store.SaveVerificationCode(ctx, user.ID, hashVerificationCode(s.cfg.JWTSecret, user.ID, code), s.cfg.VerificationCodeTTL); err != nil {
		slog.Error("verification code save", "user_id", user.ID, "err", err)
		return false
	}
	if err := s.mailer.SendVerificationCode(ctx, user.Email, code); err != nil {
		slog.Error("verification email send", "user_id", user.ID, "email", user.Email, "err", err)
		return false
	}
	return true
}

// Login — вход в приложение. Только клиент и бариста: владельцы заходят
// в кабинет на сайте (LoginOwner), роль owner в приложении не существует.
func (s *Service) Login(ctx context.Context, email, password string) (User, TokenPair, error) {
	user, pair, err := s.login(ctx, email, password)
	if err != nil {
		return User{}, TokenPair{}, err
	}
	if user.Role == "owner" {
		return User{}, TokenPair{}, ErrOwnerSiteOnly
	}
	return user, pair, nil
}

// LoginOwner — вход владельца в кабинет на сайте. Не-владельцам отказ.
func (s *Service) LoginOwner(ctx context.Context, email, password string) (User, TokenPair, error) {
	user, pair, err := s.login(ctx, email, password)
	if err != nil {
		return User{}, TokenPair{}, err
	}
	if user.Role != "owner" {
		return User{}, TokenPair{}, ErrNotOwner
	}
	return user, pair, nil
}

// login — общая проверка email+пароль и выдача пары токенов.
func (s *Service) login(ctx context.Context, email, password string) (User, TokenPair, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	user, passwordHash, _, verified, err := s.store.UserByEmail(ctx, email)
	if err != nil {
		return User{}, TokenPair{}, err
	}
	if passwordHash == "" {
		return User{}, TokenPair{}, ErrNoPassword // Yandex-only аккаунт
	}
	ok, err := VerifyPassword(password, passwordHash)
	if err != nil {
		return User{}, TokenPair{}, err
	}
	if !ok {
		return User{}, TokenPair{}, ErrBadCredentials
	}
	// Пароль верный, но email не подтверждён: вход закрыт до ввода кода.
	if !verified {
		return User{}, TokenPair{}, ErrEmailNotVerified
	}
	pair, err := s.issuePair(ctx, user)
	if err != nil {
		return User{}, TokenPair{}, err
	}
	return user, pair, nil
}

// Refresh проверяет refresh-токен и ротирует его: старый отзывается, новый выдаётся.
// Повторное использование отозванного токена отзывает все токены пользователя.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (User, TokenPair, error) {
	row, err := s.store.RefreshTokenByHash(ctx, HashRefreshToken(refreshToken))
	if err != nil {
		return User{}, TokenPair{}, err
	}

	if row.Revoked {
		// re-use украденного токена: сжигаем всю сессию пользователя.
		_ = s.store.RevokeAllForUser(ctx, row.UserID)
		return User{}, TokenPair{}, ErrRefreshReuse
	}
	if time.Now().After(row.ExpiresAt) {
		return User{}, TokenPair{}, ErrRefreshInvalid
	}

	user, err := s.userByID(ctx, row.UserID)
	if err != nil {
		return User{}, TokenPair{}, err
	}

	newToken, newHash, err := NewRefreshToken()
	if err != nil {
		return User{}, TokenPair{}, err
	}
	if err := s.store.RotateRefresh(ctx, row.ID, row.UserID, newHash, s.cfg.RefreshTokenTTL); err != nil {
		return User{}, TokenPair{}, err
	}

	access, err := IssueAccessToken(s.cfg, user.ID, user.Role, user.Email)
	if err != nil {
		return User{}, TokenPair{}, err
	}
	return user, TokenPair{AccessToken: access, RefreshToken: newToken, TokenType: "Bearer"}, nil
}

// Logout отзывает refresh-токен (access живёт до конца своего TTL).
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	return s.store.RevokeRefresh(ctx, HashRefreshToken(refreshToken))
}

// LoginYandex — вход/регистрация через Yandex ID. Связка по email:
//  1. есть пользователь с этим yandex_id — вход;
//  2. есть пользователь с этим email без yandex_id — привязываем и входим;
//  3. пользователя нет — регистрируем клиента с yandex_id.
//
// Яндекс — дополнительный способ входа только для клиентов приложения:
// владелец (owner) и бариста получают 409 и входят по паролю.
func (s *Service) LoginYandex(ctx context.Context, yandexID, email string) (User, TokenPair, error) {
	if yandexID == "" || email == "" {
		return User{}, TokenPair{}, httpx.NewError(400, "yandex_profile", "Yandex не вернул id/email")
	}
	email = strings.ToLower(strings.TrimSpace(email))

	user, found, err := s.store.UserByYandexID(ctx, yandexID)
	if err != nil {
		return User{}, TokenPair{}, err
	}
	linkNeeded := false
	if !found {
		existing, ok, err := s.store.FindByEmail(ctx, email)
		if err != nil {
			return User{}, TokenPair{}, err
		}
		if ok {
			// Уже есть аккаунт с этим email — свяжем его с Яндексом (после
			// проверки роли), чтобы пароль и Яндекс вели в одну учётку.
			user = existing
			linkNeeded = true
		} else {
			user, err = s.store.CreateUser(ctx, email, "client", "", yandexID, true)
			if err != nil {
				return User{}, TokenPair{}, err
			}
		}
	}

	// Роль проверяется всегда, а не только при первой привязке: аккаунт мог быть
	// связан клиентом и позже стать баристой. Яндекс — вход только для клиентов.
	switch {
	case user.Role == "owner":
		return User{}, TokenPair{}, errYandexOwner
	case user.Role != "client":
		return User{}, TokenPair{}, errYandexBarista
	}

	if linkNeeded {
		if err := s.store.LinkYandex(ctx, user.ID, yandexID); err != nil {
			return User{}, TokenPair{}, err
		}
	}

	pair, err := s.issuePair(ctx, user)
	if err != nil {
		return User{}, TokenPair{}, err
	}
	return user, pair, nil
}

// Ошибки ролей для входа через Яндекс: способ дополнительный и только для клиентов.
var (
	errYandexOwner   = httpx.NewError(409, "yandex_owner", "аккаунт владельца не связывается через Yandex ID")
	errYandexBarista = httpx.NewError(409, "yandex_barista", "вход через Яндекс — только для клиентов; бариста входит по email и паролю")
)

// issuePair — access + свежий refresh в БД.
func (s *Service) issuePair(ctx context.Context, user User) (TokenPair, error) {
	refresh, hash, err := NewRefreshToken()
	if err != nil {
		return TokenPair{}, err
	}
	if err := s.store.InsertRefreshToken(ctx, user.ID, hash, s.cfg.RefreshTokenTTL); err != nil {
		return TokenPair{}, err
	}
	access, err := IssueAccessToken(s.cfg, user.ID, user.Role, user.Email)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{AccessToken: access, RefreshToken: refresh, TokenType: "Bearer"}, nil
}

func (s *Service) userByID(ctx context.Context, id string) (User, error) {
	u, _, _, err := s.store.UserByID(ctx, id)
	return u, err
}

// newVerificationCode — 6 цифр из crypto/rand (не math/rand).
func newVerificationCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("auth: verification code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// hashVerificationCode — HMAC-SHA256 с JWT_SECRET: в БД нет открытого кода,
// а без секрета перебор 10^6 вариантов по украденной таблице бессмысленен.
func hashVerificationCode(secret, userID, code string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("coffeely/email-code/v1:" + userID + ":" + code))
	return hex.EncodeToString(mac.Sum(nil))
}
