package order

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the order handler on the given router group,
// wrapping every route in the supplied auth middleware.
// Callers pass an /api/v1 group; effective paths become /api/v1/orders/...
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, authMW gin.HandlerFunc) {
	g := rg.Group("/orders", authMW)
	g.POST("", h.Create)
	g.GET("", h.List)
	g.GET("/:id", h.Get)
	g.PUT("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
}
