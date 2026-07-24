package auth

import "github.com/gin-gonic/gin"

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/auth")
	g.POST("/signup", h.Signup)
	g.POST("/login", h.Login)
}
