package database

import (
	"context"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver
	"github.com/jmoiron/sqlx"

	"github.com/claudiovaldi/order-mock-payment/internal/config"
)

type Postgres struct {
	DB *sqlx.DB
}

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

func (p *Postgres) HealthCheck(ctx context.Context) error {
	return p.DB.PingContext(ctx)
}

func (p *Postgres) Close() error {
	return p.DB.Close()
}
