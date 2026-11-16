// Package admin — кабинет владельца на сайте: свои точки и рабочие аккаунты
// бариста. Роль owner существует только на сайте (в приложение владельцы
// не входят), поэтому вся админка живёт в отдельных эндпоинтах /admin/*.
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Incept10n/coffeely/backend/internal/httpx"
)

var (
	ErrForeignPoint = httpx.NewError(403, "point_foreign", "точка принадлежит другому владельцу")
	ErrBadCapacity  = httpx.NewError(400, "bad_capacity", "некорректные настройки ёмкости")
	ErrEmailTaken   = httpx.NewError(409, "email_taken", "email занят")
)

// Point — точка кофейни владельца.
type Point struct {
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Address  string          `json:"address"`
	Capacity json.RawMessage `json:"capacity_config"`
}

// Barista — рабочий аккаунт, созданный владельцем в кабинете.
type Barista struct {
	ID         string `json:"id"`
	Email      string `json:"email"`
	PointID    string `json:"point_id"`
	PointTitle string `json:"point_title"`
	CreatedAt  string `json:"created_at"`
}

// MenuItem — позиция меню точки.
type MenuItem struct {
	Title       string `json:"title"`
	PriceMinor  int    `json:"price_minor"`
	PrepSeconds int    `json:"prep_seconds"`
}

// Store — SQL для точек и бариста владельца.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// CreatePoint — точка + меню одной транзакцией.
func (s *Store) CreatePoint(ctx context.Context, ownerID, title, address string, capacity json.RawMessage, menu []MenuItem) (Point, error) {
	var p Point
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return p, fmt.Errorf("admin: point tx: %w", err)
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `
		INSERT INTO points (title, address, capacity_config, owner_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, title, address, capacity_config`,
		title, address, capacity, ownerID,
	).Scan(&p.ID, &p.Title, &p.Address, &p.Capacity)
	if err != nil {
		return p, fmt.Errorf("admin: point insert: %w", err)
	}
	for _, mi := range menu {
		if _, err := tx.Exec(ctx, `
			INSERT INTO menu_items (point_id, title, price_minor, prep_seconds)
			VALUES ($1, $2, $3, $4)`,
			p.ID, mi.Title, mi.PriceMinor, mi.PrepSeconds,
		); err != nil {
			return p, fmt.Errorf("admin: menu insert: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return p, fmt.Errorf("admin: point commit: %w", err)
	}
	return p, nil
}

// ListPoints — точки владельца.
func (s *Store) ListPoints(ctx context.Context, ownerID string) ([]Point, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, title, address, capacity_config
		FROM points WHERE owner_id = $1
		ORDER BY created_at`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("admin: points list: %w", err)
	}
	defer rows.Close()
	points := []Point{}
	for rows.Next() {
		var p Point
		if err := rows.Scan(&p.ID, &p.Title, &p.Address, &p.Capacity); err != nil {
			return nil, fmt.Errorf("admin: points scan: %w", err)
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// PointOwnedBy — точка принадлежит владельцу? (перед привязкой баристы)
func (s *Store) PointOwnedBy(ctx context.Context, ownerID, pointID string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM points WHERE id = $1 AND owner_id = $2)`,
		pointID, ownerID,
	).Scan(&ok)
	return ok, err
}

// CreateBarista — рабочий аккаунт баристы + привязка к точке владельца (tx).
func (s *Store) CreateBarista(ctx context.Context, email, passwordHash, pointID string) (Barista, error) {
	var b Barista
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return b, fmt.Errorf("admin: barista tx: %w", err)
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, role, password_hash) VALUES ($1, 'barista', $2)
		RETURNING id`, email, passwordHash,
	).Scan(&b.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return b, ErrEmailTaken
		}
		return b, fmt.Errorf("admin: barista insert: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO barista_points (user_id, point_id) VALUES ($1, $2)`,
		b.ID, pointID,
	); err != nil {
		return b, fmt.Errorf("admin: barista bind: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT title FROM points WHERE id = $1`, pointID,
	).Scan(&b.PointTitle); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return b, ErrForeignPoint
		}
		return b, fmt.Errorf("admin: barista point: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT created_at::text FROM barista_points WHERE user_id = $1`, b.ID,
	).Scan(&b.CreatedAt); err != nil {
		return b, fmt.Errorf("admin: barista created: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return b, fmt.Errorf("admin: barista commit: %w", err)
	}
	b.Email, b.PointID = email, pointID
	return b, nil
}

// ListBaristas — все рабочие аккаунты всех точек владельца.
func (s *Store) ListBaristas(ctx context.Context, ownerID string) ([]Barista, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.email, bp.point_id, p.title, bp.created_at::text
		FROM barista_points bp
		JOIN users u ON u.id = bp.user_id
		JOIN points p ON p.id = bp.point_id
		WHERE p.owner_id = $1
		ORDER BY bp.created_at DESC`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("admin: baristas list: %w", err)
	}
	defer rows.Close()
	list := []Barista{}
	for rows.Next() {
		var b Barista
		if err := rows.Scan(&b.ID, &b.Email, &b.PointID, &b.PointTitle, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("admin: baristas scan: %w", err)
		}
		list = append(list, b)
	}
	return list, rows.Err()
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
