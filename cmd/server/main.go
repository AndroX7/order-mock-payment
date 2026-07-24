package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/claudiovaldi/order-mock-payment/internal/auth"
	"github.com/claudiovaldi/order-mock-payment/internal/cache"
	"github.com/claudiovaldi/order-mock-payment/internal/config"
	"github.com/claudiovaldi/order-mock-payment/internal/database"
	"github.com/claudiovaldi/order-mock-payment/internal/logger"
	"github.com/claudiovaldi/order-mock-payment/internal/middleware"
	"github.com/claudiovaldi/order-mock-payment/internal/order"
	"github.com/claudiovaldi/order-mock-payment/internal/payment"
	"github.com/claudiovaldi/order-mock-payment/internal/server"
	"github.com/claudiovaldi/order-mock-payment/internal/upload"
	"github.com/claudiovaldi/order-mock-payment/internal/webhook"
)

const startupTimeout = 30 * time.Second

var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := logger.New(cfg.App.Env, cfg.App.LogLevel)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	startCtx, startCancel := context.WithTimeout(ctx, startupTimeout)
	defer startCancel()

	pg, err := database.New(startCtx, cfg.Postgres)
	if err != nil {
		return err
	}
	defer func() {
		if err := pg.Close(); err != nil {
			log.Error("postgres close failed", "error", err)
		}
	}()

	rd, err := cache.New(startCtx, cfg.Redis)
	if err != nil {
		return err
	}
	defer func() {
		if err := rd.Close(); err != nil {
			log.Error("redis close failed", "error", err)
		}
	}()

	startCancel()

	tokenSvc := auth.NewHMACTokenService(cfg.JWT.Secret, cfg.JWT.TTL)
	authMW := middleware.RequireAuth(tokenSvc, log)

	authRepo := auth.NewPostgresRepository(pg.DB)
	authSvc := auth.NewService(authRepo, auth.BcryptHasher{}, tokenSvc)
	authHandler := auth.NewHandler(authSvc)

	orderRepo := order.NewPostgresRepository(pg.DB)
	orderSvc := order.NewService(orderRepo)
	orderHandler := order.NewHandler(orderSvc)

	paymentGateway := payment.NewMockGateway()
	paymentRepo := payment.NewPostgresRepository(pg.DB)
	paymentSvc := payment.NewService(paymentRepo, orderSvc, paymentGateway)
	paymentHandler := payment.NewHandler(paymentSvc)

	webhookVerifier := webhook.NewHMACSignatureVerifier(cfg.Webhook.Secret)
	webhookSvc := webhook.NewService(webhookVerifier, paymentSvc)
	webhookHandler := webhook.NewHandler(webhookSvc)

	uploadStorage := upload.NewLocalStorage(cfg.Upload.BaseDir)
	uploadRepo := upload.NewPostgresRepository(pg.DB)
	uploadSvc := upload.NewService(uploadRepo, uploadStorage, orderSvc, cfg.Upload.MaxSize)
	uploadHandler := upload.NewHandler(uploadSvc)

	srv := server.New(server.Deps{
		Config:         cfg,
		Logger:         log,
		Postgres:       pg,
		Redis:          rd,
		AuthHandler:    authHandler,
		OrderHandler:   orderHandler,
		PaymentHandler: paymentHandler,
		WebhookHandler: webhookHandler,
		UploadHandler:  uploadHandler,
		AuthMiddleware: authMW,
		Version:        version,
	})

	log.Info("server starting",
		"env", cfg.App.Env,
		"port", cfg.App.HTTPPort,
		"version", version,
	)

	return srv.Run(ctx)
}
