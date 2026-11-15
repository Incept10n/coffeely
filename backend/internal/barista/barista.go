// Package barista — коды приглашений и привязка баристы к точке (веха M2).
package barista

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Incept10n/coffeely/backend/internal/httpx"
)

var (
	ErrCodeInvalid = httpx.NewError(404, "invite_invalid", "код приглашения не найден")
	ErrCodeUsed    = httpx.NewError(410, "invite_used", "код уже использован")
	ErrCodeExpired = httpx.NewError(410, "invite_expired", "код истёк")
	ErrAlreadyBarista = httpx.NewError(409, "already_barista", "пользователь уже привязан к точке")
)

// InviteCode — выданный код приглашения.
type InviteCode struct {
	Code      string
	PointID   string
	ExpiresAt time.Time
}

// Store — SQL для barista_invite_codes и barista_points.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// CreateInviteCode генерирует одноразовый код для точки.
func (s *Store) CreateInviteCode(ctx context.Context, pointID, createdBy string, ttl time.Duration) (InviteCode, error) {
	code, err := newCode()
	if err != nil {
		return InviteCode{}, err
	}
	ic := InviteCode{Code: code, PointID: pointID}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO barista_invite_codes (code, point_id, created_by, expires_at)
		VALUES ($1, $2, $3, now() + $4::interval)
		RETURNING expires_at`,
		code, pointID, createdBy, fmt.Sprintf("%d seconds", int(ttl.Seconds())),
	).Scan(&ic.ExpiresAt)
	if err != nil {
		return InviteCode{}, fmt.Errorf("barista: create invite: %w", err)
	}
	return ic, nil
}

// Claim атомарно съедает код и привязывает баристу к точке: код одноразовый,
// у баристы одна точка (повторный claim перезаписывает привязку).
func (s *Store) Claim(ctx context.Context, userID, code string) (pointID string, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("barista: claim tx: %w", err)
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `
		UPDATE barista_invite_codes
		SET used_by = $1, used_at = now()
		WHERE code = $2 AND used_by IS NULL AND expires_at > now()
		RETURNING point_id`,
		userID, code,
	).Scan(&pointID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Код не съелся: разбираемся, почему, для точной ошибки.
		var expires time.Time
		var used bool
		qErr := tx.QueryRow(ctx, `
			SELECT expires_at, used_by IS NOT NULL
			FROM barista_invite_codes WHERE code = $1`, code,
		).Scan(&expires, &used)
		switch {
		case errors.Is(qErr, pgx.ErrNoRows):
			return "", ErrCodeInvalid
		case used:
			return "", ErrCodeUsed
		default:
			return "", ErrCodeExpired
		}
	case err != nil:
		return "", fmt.Errorf("barista: claim update: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO barista_points (user_id, point_id) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET point_id = EXCLUDED.point_id, created_at = now()`,
		userID, pointID,
	); err != nil {
		return "", fmt.Errorf("barista: link point: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE users SET role = 'barista' WHERE id = $1 AND role <> 'owner'`, userID,
	); err != nil {
		return "", fmt.Errorf("barista: upgrade role: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("barista: claim commit: %w", err)
	}
	return pointID, nil
}

// newCode — одноразовый код в Crockford base32 (без I/L/O/U в буквах).
func newCode() (string, error) {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("barista: code: %w", err)
	}
	return base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding).EncodeToString(raw), nil
}
