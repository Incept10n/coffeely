package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Incept10n/coffeely/backend/internal/httpx"
)

// User — запись пользователя без секретов.
type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

// TokenPair — access + refresh, выдаваемые вместе.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
}

// Ошибки домена (сравниваются через errors.Is по коду httpx.Error).
var (
	ErrEmailTaken     = httpx.NewError(409, "email_taken", "email занят")
	ErrBadCredentials = httpx.NewError(401, "bad_credentials", "неверные учётные данные")
	ErrNoPassword     = httpx.NewError(401, "no_password", "вход по паролю недоступен")
	ErrRefreshReuse   = httpx.NewError(401, "refresh_reuse", "refresh-токен уже использован")
	ErrRefreshInvalid = httpx.NewError(401, "refresh_invalid", "refresh-токен недействителен")
)

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Store — SQL-запросы к users и refresh_tokens.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) CreateUser(ctx context.Context, email, role, passwordHash, yandexID string) (User, error) {
	u := User{Email: email, Role: role}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (email, role, password_hash, yandex_id)
		VALUES ($1, $2, $3, NULLIF($4, ''))
		RETURNING id, email, role`,
		email, role, passwordHash, yandexID,
	).Scan(&u.ID, &u.Email, &u.Role)
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("auth: create user: %w", err)
	}
	return u, nil
}

func (s *Store) UserByEmail(ctx context.Context, email string) (User, string, string, error) {
	var u User
	var passwordHash, yandexID *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, role, password_hash, yandex_id FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.Role, &passwordHash, &yandexID)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", "", ErrBadCredentials
	}
	if err != nil {
		return User{}, "", "", fmt.Errorf("auth: user by email: %w", err)
	}
	if passwordHash != nil {
		return u, *passwordHash, deref(yandexID), nil
	}
	return u, "", deref(yandexID), nil
}

// UserByYandexID ищет пользователя по связанному yandex_id.
func (s *Store) UserByYandexID(ctx context.Context, yandexID string) (User, bool, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, role FROM users WHERE yandex_id = $1`, yandexID,
	).Scan(&u.ID, &u.Email, &u.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, fmt.Errorf("auth: user by yandex id: %w", err)
	}
	return u, true, nil
}

// FindByEmail ищет пользователя без разбора секретов (для связывания OAuth).
func (s *Store) FindByEmail(ctx context.Context, email string) (User, bool, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, role FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, fmt.Errorf("auth: find by email: %w", err)
	}
	return u, true, nil
}

func (s *Store) UserByID(ctx context.Context, id string) (User, string, string, error) {
	var u User
	var passwordHash, yandexID *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, role, password_hash, yandex_id FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.Role, &passwordHash, &yandexID)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", "", ErrBadCredentials
	}
	if err != nil {
		return User{}, "", "", fmt.Errorf("auth: user by id: %w", err)
	}
	return u, deref(passwordHash), deref(yandexID), nil
}

// LinkYandex связывает yandex_id с существующим пользователем по email.
func (s *Store) LinkYandex(ctx context.Context, userID, yandexID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET yandex_id = $2 WHERE id = $1 AND yandex_id IS NULL`,
		userID, yandexID,
	)
	if err != nil {
		return fmt.Errorf("auth: link yandex: %w", err)
	}
	return nil
}

// InsertRefreshToken сохраняет хеш refresh-токена.
func (s *Store) InsertRefreshToken(ctx context.Context, userID, tokenHash string, ttl time.Duration) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, now() + $3::interval)`,
		userID, tokenHash, fmt.Sprintf("%d seconds", int(ttl.Seconds())),
	)
	if err != nil {
		return fmt.Errorf("auth: insert refresh: %w", err)
	}
	return nil
}

// RefreshTokenRow — запись refresh-токена из БД.
type RefreshTokenRow struct {
	ID        string
	UserID    string
	ExpiresAt time.Time
	Revoked   bool
}

func (s *Store) RefreshTokenByHash(ctx context.Context, tokenHash string) (RefreshTokenRow, error) {
	var row RefreshTokenRow
	// revoked_at IS NOT NULL — булево выражение, NULL быть не может.
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, expires_at, revoked_at IS NOT NULL
		FROM refresh_tokens WHERE token_hash = $1`, tokenHash,
	).Scan(&row.ID, &row.UserID, &row.ExpiresAt, &row.Revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return RefreshTokenRow{}, ErrRefreshInvalid
	}
	if err != nil {
		return RefreshTokenRow{}, fmt.Errorf("auth: refresh by hash: %w", err)
	}
	return row, nil
}

// RotateRefresh атомарно: отзывает старый токен, создаёт новый, возвращает его id.
func (s *Store) RotateRefresh(ctx context.Context, oldID, userID, newHash string, ttl time.Duration) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("auth: rotate tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`,
		oldID,
	); err != nil {
		return fmt.Errorf("auth: revoke old: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, replaced_by)
		VALUES ($1, $2, now() + $3::interval, $4)`,
		userID, newHash, fmt.Sprintf("%d seconds", int(ttl.Seconds())), oldID,
	); err != nil {
		return fmt.Errorf("auth: insert rotated: %w", err)
	}
	return tx.Commit(ctx)
}

// RevokeRefresh отзывает конкретный refresh-токен (logout).
func (s *Store) RevokeRefresh(ctx context.Context, tokenHash string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`,
		tokenHash,
	)
	if err != nil {
		return fmt.Errorf("auth: revoke refresh: %w", err)
	}
	return nil
}

// RevokeAllForUser — реакция на повторное использование отозванного токена.
func (s *Store) RevokeAllForUser(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("auth: revoke all: %w", err)
	}
	return nil
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
