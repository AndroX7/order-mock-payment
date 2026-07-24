// Package database bootstraps the PostgreSQL connection pool.
package database

import (
	"context"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver
	"github.com/jmoiron/sqlx"

	"github.com/claudiovaldi/order-mock-payment/internal/config"
)

// Postgres owns the pooled sqlx handle. Repositories depend on the *sqlx.DB directly.
type Postgres struct {
	DB *sqlx.DB
}

// New opens a pooled connection, applies pool limits, and verifies with a ping.
func New(ctx context.Context, cfg config.Postgres) (*Postgres, error) {
	db, err := sqlx.Open("pgx", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("postgres open: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}

	return &Postgres{DB: db}, nil
}

// HealthCheck verifies the DB is reachable. Callers should pass a bounded context.
func (p *Postgres) HealthCheck(ctx context.Context) error {
	return p.DB.PingContext(ctx)
}

func (p *Postgres) Close() error {
	return p.DB.Close()
}
