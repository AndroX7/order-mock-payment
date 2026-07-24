package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
)

// Repository is the minimal persistence surface the auth service needs.
// Kept small on purpose — no generic CRUD, no anticipation of future callers.
type Repository interface {
	Create(ctx context.Context, u *User) error
	GetByEmail(ctx context.Context, email string) (*User, error)
}

// PostgresRepository is the sqlx-backed implementation of Repository.
type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

const createUserQuery = `
INSERT INTO users (email, password_hash, name)
VALUES ($1, $2, $3)
RETURNING id, created_at, updated_at
`

// Create inserts a new user and populates u.ID, u.CreatedAt, u.UpdatedAt
// from the database. Returns ErrEmailAlreadyExists on a unique-email conflict.
func (r *PostgresRepository) Create(ctx context.Context, u *User) error {
	err := r.db.QueryRowxContext(ctx, createUserQuery, u.Email, u.PasswordHash, u.Name).
		Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrEmailAlreadyExists
		}
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

const getByEmailQuery = `
SELECT id, email, password_hash, name, created_at, updated_at
FROM users
WHERE email = $1
`

// GetByEmail returns the user with the given email, or ErrUserNotFound.
func (r *PostgresRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	if err := r.db.GetContext(ctx, &u, getByEmailQuery, email); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return &u, nil
}

// pgUniqueViolationCode is PostgreSQL SQLSTATE 23505 (unique_violation).
const pgUniqueViolationCode = "23505"

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolationCode
}

var _ Repository = (*PostgresRepository)(nil)
