package auth

import (
	"context"
	"fmt"
	"net/mail"
	"time"

	"github.com/Incept10n/coffeely/backend/internal/config"
	"github.com/Incept10n/coffeely/backend/internal/httpx"
)

// Service — бизнес-логика аутентификации: регистрация, вход, ротация refresh, Yandex.
type Service struct {
	store *Store
	cfg   config.Config
}

func NewService(store *Store, cfg config.Config) *Service {
	return &Service{store: store, cfg: cfg}
}

// Register создаёт клиента (role=client) и выдаёт пару токенов. Приложение
// регистрирует только клиентов: владелец регистрируется на сайте (RegisterOwner).
func (s *Service) Register(ctx context.Context, email, password string) (User, TokenPair, error) {
	return s.registerAs(ctx, email, password, "client")
}

// RegisterOwner — регистрация владельца кофейни на сайте (личный кабинет).
func (s *Service) RegisterOwner(ctx context.Context, email, password string) (User, TokenPair, error) {
	return s.registerAs(ctx, email, password, "owner")
}

func (s *Service) registerAs(ctx context.Context, email, password, role string) (User, TokenPair, error) {
	if _, err := mail.ParseAddress(email); err != nil {
		return User{}, TokenPair{}, fmt.Errorf("неверный email: %w", ErrBadCredentials)
	}
	if len(password) < 8 {
		return User{}, TokenPair{}, fmt.Errorf("пароль короче 8 символов: %w", ErrBadCredentials)
	}

	hash, err := HashPassword(password)
	if err != nil {
		return User{}, TokenPair{}, err
	}
	user, err := s.store.CreateUser(ctx, email, role, hash, "")
	if err != nil {
		return User{}, TokenPair{}, err
	}
	pair, err := s.issuePair(ctx, user)
	if err != nil {
		return User{}, TokenPair{}, err
	}
	return user, pair, nil
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
	user, passwordHash, yandexID, err := s.store.UserByEmail(ctx, email)
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
	_ = yandexID
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
func (s *Service) LoginYandex(ctx context.Context, yandexID, email string) (User, TokenPair, error) {
	if yandexID == "" || email == "" {
		return User{}, TokenPair{}, httpx.NewError(400, "yandex_profile", "Yandex не вернул id/email")
	}

	user, found, err := s.store.UserByYandexID(ctx, yandexID)
	if err != nil {
		return User{}, TokenPair{}, err
	}
	if !found {
		existing, ok, err := s.store.FindByEmail(ctx, email)
		if err != nil {
			return User{}, TokenPair{}, err
		}
		switch {
		case !ok:
			user, err = s.store.CreateUser(ctx, email, "client", "", yandexID)
			if err != nil {
				return User{}, TokenPair{}, err
			}
		case existing.Role == "owner":
			// Владелец не входит в приложение и не связывается через Yandex ID —
			// кабинет только по email+пароль на сайте.
			return User{}, TokenPair{}, httpx.NewError(409, "yandex_owner", "аккаунт владельца не связывается через Yandex ID")
		default:
			if err := s.store.LinkYandex(ctx, existing.ID, yandexID); err != nil {
				return User{}, TokenPair{}, err
			}
			user = existing
			user.Role = existing.Role
		}
	}

	pair, err := s.issuePair(ctx, user)
	if err != nil {
		return User{}, TokenPair{}, err
	}
	return user, pair, nil
}

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
