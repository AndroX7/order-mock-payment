package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/claudiovaldi/order-mock-payment/internal/auth"
	"github.com/claudiovaldi/order-mock-payment/internal/cache"
	"github.com/claudiovaldi/order-mock-payment/internal/config"
	"github.com/claudiovaldi/order-mock-payment/internal/database"
	"github.com/claudiovaldi/order-mock-payment/internal/order"
	"github.com/claudiovaldi/order-mock-payment/internal/payment"
	"github.com/claudiovaldi/order-mock-payment/internal/upload"
	"github.com/claudiovaldi/order-mock-payment/internal/webhook"
)

type Deps struct {
	Config         *config.Config
	Logger         *slog.Logger
	Postgres       *database.Postgres
	Redis          *cache.Redis
	AuthHandler    *auth.Handler
	OrderHandler   *order.Handler
	PaymentHandler *payment.Handler
	WebhookHandler *webhook.Handler
	UploadHandler  *upload.Handler
	AuthMiddleware gin.HandlerFunc
	Version        string
}

type Server struct {
	httpSrv         *http.Server
	logger          *slog.Logger
	shutdownTimeout time.Duration
}

func New(deps Deps) *Server {
	if deps.Config.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(slogRecovery(deps.Logger))

	registerRootRoute(router, deps.Version)
	registerHealthRoutes(router, deps.Postgres, deps.Redis)

	api := router.Group("/api/v1")
	deps.AuthHandler.RegisterRoutes(api)
	deps.OrderHandler.RegisterRoutes(api, deps.AuthMiddleware)
	deps.PaymentHandler.RegisterRoutes(api, deps.AuthMiddleware)
	deps.UploadHandler.RegisterRoutes(api, deps.AuthMiddleware)

	webhooks := router.Group("/webhooks")
	deps.WebhookHandler.RegisterRoutes(webhooks)

	httpSrv := &http.Server{
		Addr:              net.JoinHostPort("", strconv.Itoa(deps.Config.App.HTTPPort)),
		Handler:           router,
		ReadTimeout:       deps.Config.App.ReadTimeout,
		ReadHeaderTimeout: deps.Config.App.ReadTimeout,
		WriteTimeout:      deps.Config.App.WriteTimeout,
		IdleTimeout:       deps.Config.App.IdleTimeout,
	}

	return &Server{
		httpSrv:         httpSrv,
		logger:          deps.Logger,
		shutdownTimeout: deps.Config.App.ShutdownTimeout,
	}
}

func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("http listening", "addr", s.httpSrv.Addr)
		if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http listen: %w", err)
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		s.logger.Info("shutdown signal received")
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()

	if err := s.httpSrv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http shutdown: %w", err)
	}

	s.logger.Info("server stopped cleanly")
	return nil
}

func registerRootRoute(r *gin.Engine, version string) {
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": "order-mock-payment",
			"version": version,
			"status":  "running",
		})
	})
}

func slogRecovery(log *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, err any) {
		log.Error("panic recovered",
			"error", fmt.Sprint(err),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
		)
		c.AbortWithStatus(http.StatusInternalServerError)
	})
}
