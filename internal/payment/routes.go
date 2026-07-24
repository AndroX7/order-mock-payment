package payment

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, authMW gin.HandlerFunc) {
	g := rg.Group("/payments", authMW)
	g.POST("", h.Create)
	g.GET("/:id", h.Get)
}
