package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"

	"github.com/AndroX7/order-mock-payment/internal/cache"
	"github.com/AndroX7/order-mock-payment/internal/config"
	"github.com/AndroX7/order-mock-payment/internal/database"
	"github.com/AndroX7/order-mock-payment/internal/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := logger.New(cfg.Log.Level, cfg.Log.Format)
	slog.SetDefault(log)

	log.Info("starting service",
		slog.String("env", cfg.App.Env),
		slog.Int("port", cfg.HTTP.Port),
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	initCtx, cancelInit := context.WithTimeout(ctx, 10*time.Second)
	defer cancelInit()

	db, err := database.NewPostgres(initCtx, cfg.Postgres)
	if err != nil {
		return fmt.Errorf("init postgres: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Error("postgres close", slog.Any("err", err))
		}
	}()
	log.Info("postgres connected")

	rdb, err := cache.NewRedis(initCtx, cfg.Redis)
	if err != nil {
		return fmt.Errorf("init redis: %w", err)
	}
	defer func() {
		if err := rdb.Close(); err != nil {
			log.Error("redis close", slog.Any("err", err))
		}
	}()
	log.Info("redis connected")

	router := newRouter(cfg, db, rdb)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTP.Port),
		Handler:      router,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("http server listening", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		log.Info("shutdown signal received")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancelShutdown()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("http shutdown", slog.Any("err", err))
		return fmt.Errorf("shutdown: %w", err)
	}

	log.Info("shutdown complete")
	return nil
}

func newRouter(cfg *config.Config, db *sqlx.DB, rdb *redis.Client) *gin.Engine {
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/readyz", func(c *gin.Context) {
		checkCtx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		checks := map[string]string{}
		ready := true

		if err := db.PingContext(checkCtx); err != nil {
			checks["postgres"] = "down"
			ready = false
			slog.Warn("readyz: postgres ping failed", slog.Any("err", err))
		} else {
			checks["postgres"] = "ok"
		}

		if err := rdb.Ping(checkCtx).Err(); err != nil {
			checks["redis"] = "down"
			ready = false
			slog.Warn("readyz: redis ping failed", slog.Any("err", err))
		} else {
			checks["redis"] = "ok"
		}

		status := http.StatusOK
		if !ready {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"ready": ready, "checks": checks})
	})

	return r
}
