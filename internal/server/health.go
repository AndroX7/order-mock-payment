package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/claudiovaldi/order-mock-payment/internal/cache"
	"github.com/claudiovaldi/order-mock-payment/internal/database"
)

const healthCheckTimeout = 2 * time.Second

func registerHealthRoutes(r *gin.Engine, pg *database.Postgres, rd *cache.Redis) {
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/readyz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), healthCheckTimeout)
		defer cancel()

		checks := gin.H{
			"postgres": "ok",
			"redis":    "ok",
		}
		ready := true

		if err := pg.HealthCheck(ctx); err != nil {
			checks["postgres"] = err.Error()
			ready = false
		}
		if err := rd.HealthCheck(ctx); err != nil {
			checks["redis"] = err.Error()
			ready = false
		}

		status := http.StatusOK
		state := "ready"
		if !ready {
			status = http.StatusServiceUnavailable
			state = "not_ready"
		}
		c.JSON(status, gin.H{
			"status": state,
			"checks": checks,
		})
	})
}
