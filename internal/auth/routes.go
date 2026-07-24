package auth

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the auth handler's endpoints on the given router
// group. Callers pass an /api/v1 group; effective paths become /api/v1/auth/...
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/auth")
	g.POST("/signup", h.Signup)
	g.POST("/login", h.Login)
}
