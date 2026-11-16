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
	// Роль owner существует только на сайте: приложение клиентское.
	ErrOwnerSiteOnly = httpx.NewError(403, "owner_site_only", "вход владельцев — на сайте coffeely.inceptech.ru/owner")
	ErrNotOwner      = httpx.NewError(403, "not_owner", "кабинет доступен только владельцам")

	// Подтверждение email после регистрации в приложении.
	ErrEmailNotVerified = httpx.NewError(403, "email_not_verified", "подтвердите email кодом из письма")
	ErrCodeNotFound     = httpx.NewError(400, "invalid_code", "неверный код подтверждения")
	ErrCodeExpired      = httpx.NewError(410, "code_expired", "код истёк — запросите новый")
	ErrTooManyAttempts  = httpx.NewError(429, "too_many_attempts", "слишком много попыток — запросите новый код")
	ErrResendTooSoon    = httpx.NewError(429, "resend_too_soon", "код уже отправлен — подождите минуту")
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

func (s *Store) CreateUser(ctx context.Context, email, role, passwordHash, yandexID string, verified bool) (User, error) {
	u := User{Email: email, Role: role}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (email, role, password_hash, yandex_id, email_verified_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), CASE WHEN $5 THEN now() ELSE NULL END)
		RETURNING id, email, role`,
		email, role, passwordHash, yandexID, verified,
	).Scan(&u.ID, &u.Email, &u.Role)
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("auth: create user: %w", err)
	}
	return u, nil
}

func (s *Store) UserByEmail(ctx context.Context, email string) (User, string, string, bool, error) {
	var u User
	var passwordHash, yandexID *string
	var verified bool
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, role, password_hash, yandex_id, email_verified_at IS NOT NULL
		FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.Role, &passwordHash, &yandexID, &verified)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", "", false, ErrBadCredentials
	}
	if err != nil {
		return User{}, "", "", false, fmt.Errorf("auth: user by email: %w", err)
	}
	return u, deref(passwordHash), deref(yandexID), verified, nil
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

// VerificationCode — активный код подтверждения email (без открытого значения).
type VerificationCode struct {
	ID        string
	CodeHash  string
	ExpiresAt time.Time
	Attempts  int
}

// LastVerificationCodeAt — время последней отправки кода: защита от спама
// повторными запросами. ok=false — кодов ещё не было.
func (s *Store) LastVerificationCodeAt(ctx context.Context, userID string) (time.Time, bool, error) {
	var at time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT created_at FROM email_verification_codes
		 WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1`, userID,
	).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("auth: last code: %w", err)
	}
	return at, true, nil
}

// SaveVerificationCode заменяет активный код пользователя новым.
func (s *Store) SaveVerificationCode(ctx context.Context, userID, codeHash string, ttl time.Duration) (time.Time, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return time.Time{}, fmt.Errorf("auth: code tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE email_verification_codes SET consumed_at = now()
		 WHERE user_id = $1 AND consumed_at IS NULL`, userID,
	); err != nil {
		return time.Time{}, fmt.Errorf("auth: close old codes: %w", err)
	}

	var expiresAt time.Time
	if err := tx.QueryRow(ctx, `
		INSERT INTO email_verification_codes (user_id, code_hash, expires_at)
		VALUES ($1, $2, now() + $3::interval)
		RETURNING expires_at`,
		userID, codeHash, fmt.Sprintf("%d seconds", int(ttl.Seconds())),
	).Scan(&expiresAt); err != nil {
		return time.Time{}, fmt.Errorf("auth: save code: %w", err)
	}
	return expiresAt, tx.Commit(ctx)
}

// ActiveVerificationCode — последний неиспользованный код; истёкший тоже
// возвращается, чтобы сервис отличил «истёк» от «неверный».
func (s *Store) ActiveVerificationCode(ctx context.Context, userID string) (VerificationCode, error) {
	var c VerificationCode
	err := s.pool.QueryRow(ctx, `
		SELECT id, code_hash, expires_at, attempts
		FROM email_verification_codes
		WHERE user_id = $1 AND consumed_at IS NULL
		ORDER BY created_at DESC LIMIT 1`, userID,
	).Scan(&c.ID, &c.CodeHash, &c.ExpiresAt, &c.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return VerificationCode{}, ErrCodeNotFound
	}
	if err != nil {
		return VerificationCode{}, fmt.Errorf("auth: active code: %w", err)
	}
	return c, nil
}

// IncrementCodeAttempts считает попытки ввода: после лимита код бесполезен.
func (s *Store) IncrementCodeAttempts(ctx context.Context, codeID string) error {
	if _, err := s.pool.Exec(ctx,
		`UPDATE email_verification_codes SET attempts = attempts + 1 WHERE id = $1`, codeID,
	); err != nil {
		return fmt.Errorf("auth: code attempts: %w", err)
	}
	return nil
}

// MarkEmailVerified подтверждает email и закрывает текущий код.
func (s *Store) MarkEmailVerified(ctx context.Context, userID, codeID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("auth: verify tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE email_verification_codes SET consumed_at = now() WHERE id = $1`, codeID,
	); err != nil {
		return fmt.Errorf("auth: consume code: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE users SET email_verified_at = now() WHERE id = $1 AND email_verified_at IS NULL`, userID,
	); err != nil {
		return fmt.Errorf("auth: mark verified: %w", err)
	}
	return tx.Commit(ctx)
}

// EmailVerified проверяет, подтверждён ли email пользователя.
func (s *Store) EmailVerified(ctx context.Context, userID string) (bool, error) {
	var verified bool
	err := s.pool.QueryRow(ctx,
		`SELECT email_verified_at IS NOT NULL FROM users WHERE id = $1`, userID,
	).Scan(&verified)
	if err != nil {
		return false, fmt.Errorf("auth: email verified: %w", err)
	}
	return verified, nil
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
