package payment

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the payment handler on the given router group,
// guarded by the auth middleware. Effective paths: /api/v1/payments/...
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, authMW gin.HandlerFunc) {
	g := rg.Group("/payments", authMW)
	g.POST("", h.Create)
	g.GET("/:id", h.Get)
}
