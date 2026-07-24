package payment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
)

// Repository is the minimal persistence surface the payment service needs.
// Ownership is enforced at the service layer (through the OrderService
// interface) — the repository stays a thin, join-free SQL layer.
type Repository interface {
	Create(ctx context.Context, p *Payment) error
	GetByID(ctx context.Context, paymentID uuid.UUID) (*Payment, error)
}

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

const createPaymentQuery = `
INSERT INTO payments (order_id, provider, provider_reference, amount, currency, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, created_at, updated_at
`

// Create inserts a new payment row. Populates p.ID, p.CreatedAt,
// p.UpdatedAt from the DB. Returns ErrDuplicatePayment on unique
// violation (one payment per order).
func (r *PostgresRepository) Create(ctx context.Context, p *Payment) error {
	err := r.db.QueryRowxContext(ctx, createPaymentQuery,
		p.OrderID, p.Provider, p.ProviderReference, p.Amount, p.Currency, p.Status,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicatePayment
		}
		return fmt.Errorf("insert payment: %w", err)
	}
	return nil
}

const getPaymentByIDQuery = `
SELECT id, order_id, provider, provider_reference, amount, currency, status, created_at, updated_at
FROM payments
WHERE id = $1
`

// GetByID returns the payment with the given id, or ErrPaymentNotFound.
// Ownership filtering is the caller's responsibility (service layer
// verifies the underlying order belongs to the requester).
func (r *PostgresRepository) GetByID(ctx context.Context, paymentID uuid.UUID) (*Payment, error) {
	var p Payment
	if err := r.db.GetContext(ctx, &p, getPaymentByIDQuery, paymentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPaymentNotFound
		}
		return nil, fmt.Errorf("get payment: %w", err)
	}
	return &p, nil
}

// pgUniqueViolationCode is PostgreSQL SQLSTATE 23505 (unique_violation).
const pgUniqueViolationCode = "23505"

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolationCode
}

var _ Repository = (*PostgresRepository)(nil)
