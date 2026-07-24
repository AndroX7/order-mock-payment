package upload

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, authMW gin.HandlerFunc) {
	g := rg.Group("/uploads", authMW)
	g.POST("", h.Create)
	g.GET("/:id", h.Get)
}
