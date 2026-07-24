package cache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/claudiovaldi/order-mock-payment/internal/config"
)

type Redis struct {
	Client *redis.Client
}

func New(ctx context.Context, cfg config.Redis) (*Redis, error) {
	client := redis.NewClient(&redis.Options{
		Addr:        cfg.Addr,
		Password:    cfg.Password,
		DB:          cfg.DB,
		DialTimeout: cfg.DialTimeout,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	return &Redis{Client: client}, nil
}

func (r *Redis) HealthCheck(ctx context.Context) error {
	return r.Client.Ping(ctx).Err()
}

func (r *Redis) Close() error {
	return r.Client.Close()
}
