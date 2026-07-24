package order

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Repository is the minimal persistence surface the order service needs.
// Every read/write is scoped by user_id; the interface makes that a hard
// contract callers cannot forget.
type Repository interface {
	Create(ctx context.Context, o *Order) error
	GetByID(ctx context.Context, userID, orderID uuid.UUID) (*Order, error)
	List(ctx context.Context, userID uuid.UUID) ([]*Order, error)
	Update(ctx context.Context, o *Order) error
	UpdateStatus(ctx context.Context, userID, orderID uuid.UUID, status string) error
	AdvanceStatus(ctx context.Context, orderID uuid.UUID, status string) error
	Delete(ctx context.Context, userID, orderID uuid.UUID) error
}

// PostgresRepository is the sqlx-backed implementation of Repository.
type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

const createOrderQuery = `
INSERT INTO orders (user_id, symbol, side, quantity, price, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, created_at, updated_at
`

// Create inserts a new order and populates o.ID, o.CreatedAt, o.UpdatedAt
// from the database.
func (r *PostgresRepository) Create(ctx context.Context, o *Order) error {
	err := r.db.QueryRowxContext(ctx, createOrderQuery,
		o.UserID, o.Symbol, o.Side, o.Quantity, o.Price, o.Status,
	).Scan(&o.ID, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert order: %w", err)
	}
	return nil
}

const getOrderByIDQuery = `
SELECT id, user_id, symbol, side, quantity, price, status, created_at, updated_at
FROM orders
WHERE user_id = $1 AND id = $2
`

// GetByID returns the order owned by userID with the given orderID, or
// ErrOrderNotFound. Not-owned resources are indistinguishable from
// non-existent ones — prevents ID enumeration.
func (r *PostgresRepository) GetByID(ctx context.Context, userID, orderID uuid.UUID) (*Order, error) {
	var o Order
	if err := r.db.GetContext(ctx, &o, getOrderByIDQuery, userID, orderID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("get order: %w", err)
	}
	return &o, nil
}

const listOrdersQuery = `
SELECT id, user_id, symbol, side, quantity, price, status, created_at, updated_at
FROM orders
WHERE user_id = $1
ORDER BY created_at DESC
`

// List returns all orders owned by userID, newest first.
func (r *PostgresRepository) List(ctx context.Context, userID uuid.UUID) ([]*Order, error) {
	var orders []*Order
	if err := r.db.SelectContext(ctx, &orders, listOrdersQuery, userID); err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	return orders, nil
}

const updateOrderQuery = `
UPDATE orders
SET symbol = $1, side = $2, quantity = $3, price = $4, updated_at = now()
WHERE user_id = $5 AND id = $6
RETURNING status, created_at, updated_at
`

// Update applies o's mutable fields to the row identified by (o.UserID, o.ID).
// Status is preserved (not touched by clients). Populates o.Status,
// o.CreatedAt, o.UpdatedAt from the DB via RETURNING.
// Returns ErrOrderNotFound if the row does not exist or is not owned.
func (r *PostgresRepository) Update(ctx context.Context, o *Order) error {
	err := r.db.QueryRowxContext(ctx, updateOrderQuery,
		o.Symbol, o.Side, o.Quantity, o.Price, o.UserID, o.ID,
	).Scan(&o.Status, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrOrderNotFound
		}
		return fmt.Errorf("update order: %w", err)
	}
	return nil
}

const updateOrderStatusQuery = `
UPDATE orders
SET status = $1, updated_at = now()
WHERE user_id = $2 AND id = $3
`

// UpdateStatus mutates only the status column. Used by cross-module callers
// (e.g. payment) that need to transition an order's lifecycle without
// touching mutable business fields. Returns ErrOrderNotFound if the row
// does not exist or is not owned.
func (r *PostgresRepository) UpdateStatus(ctx context.Context, userID, orderID uuid.UUID, status string) error {
	res, err := r.db.ExecContext(ctx, updateOrderStatusQuery, status, userID, orderID)
	if err != nil {
		return fmt.Errorf("update order status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update order status rows affected: %w", err)
	}
	if n == 0 {
		return ErrOrderNotFound
	}
	return nil
}

const advanceOrderStatusQuery = `
UPDATE orders
SET status = $1, updated_at = now()
WHERE id = $2
`

// AdvanceStatus mutates status without an ownership filter. Intended for
// system-initiated transitions (webhook → payment → order) where the
// caller has already established authorization via provider signature.
// Returns ErrOrderNotFound if the row does not exist.
func (r *PostgresRepository) AdvanceStatus(ctx context.Context, orderID uuid.UUID, status string) error {
	res, err := r.db.ExecContext(ctx, advanceOrderStatusQuery, status, orderID)
	if err != nil {
		return fmt.Errorf("advance order status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("advance order status rows affected: %w", err)
	}
	if n == 0 {
		return ErrOrderNotFound
	}
	return nil
}

const deleteOrderQuery = `
DELETE FROM orders
WHERE user_id = $1 AND id = $2
`

// Delete removes the order owned by userID with the given orderID.
// Returns ErrOrderNotFound if the row does not exist or is not owned.
func (r *PostgresRepository) Delete(ctx context.Context, userID, orderID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, deleteOrderQuery, userID, orderID)
	if err != nil {
		return fmt.Errorf("delete order: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete order rows affected: %w", err)
	}
	if n == 0 {
		return ErrOrderNotFound
	}
	return nil
}

var _ Repository = (*PostgresRepository)(nil)
