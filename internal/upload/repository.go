package upload

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type Repository interface {
	Create(ctx context.Context, u *Upload) error
	GetByID(ctx context.Context, userID, uploadID uuid.UUID) (*Upload, error)
}

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

const createUploadQuery = `
INSERT INTO uploads (order_id, filename, content_type, size, path)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, created_at
`

func (r *PostgresRepository) Create(ctx context.Context, u *Upload) error {
	err := r.db.QueryRowxContext(ctx, createUploadQuery,
		u.OrderID, u.Filename, u.ContentType, u.Size, u.Path,
	).Scan(&u.ID, &u.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert upload: %w", err)
	}
	return nil
}

const getUploadByIDQuery = `
SELECT u.id, u.order_id, u.filename, u.content_type, u.size, u.path, u.created_at
FROM uploads u
JOIN orders o ON o.id = u.order_id
WHERE u.id = $1 AND o.user_id = $2
`

func (r *PostgresRepository) GetByID(ctx context.Context, userID, uploadID uuid.UUID) (*Upload, error) {
	var u Upload
	if err := r.db.GetContext(ctx, &u, getUploadByIDQuery, uploadID, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUploadNotFound
		}
		return nil, fmt.Errorf("get upload: %w", err)
	}
	return &u, nil
}

var _ Repository = (*PostgresRepository)(nil)
