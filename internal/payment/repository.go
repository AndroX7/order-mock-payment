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

type Repository interface {
	Create(ctx context.Context, p *Payment) error
	GetByID(ctx context.Context, paymentID uuid.UUID) (*Payment, error)
	GetByProviderReference(ctx context.Context, reference string) (*Payment, error)
	UpdateStatus(ctx context.Context, paymentID uuid.UUID, status string) error
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

const getPaymentByReferenceQuery = `
SELECT id, order_id, provider, provider_reference, amount, currency, status, created_at, updated_at
FROM payments
WHERE provider_reference = $1
`

func (r *PostgresRepository) GetByProviderReference(ctx context.Context, reference string) (*Payment, error) {
	var p Payment
	if err := r.db.GetContext(ctx, &p, getPaymentByReferenceQuery, reference); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPaymentNotFound
		}
		return nil, fmt.Errorf("get payment by reference: %w", err)
	}
	return &p, nil
}

const updatePaymentStatusQuery = `
UPDATE payments
SET status = $1, updated_at = now()
WHERE id = $2
`

func (r *PostgresRepository) UpdateStatus(ctx context.Context, paymentID uuid.UUID, status string) error {
	res, err := r.db.ExecContext(ctx, updatePaymentStatusQuery, status, paymentID)
	if err != nil {
		return fmt.Errorf("update payment status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update payment status rows affected: %w", err)
	}
	if n == 0 {
		return ErrPaymentNotFound
	}
	return nil
}

const pgUniqueViolationCode = "23505"

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolationCode
}

var _ Repository = (*PostgresRepository)(nil)
