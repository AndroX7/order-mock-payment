package upload

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the upload handler on the given router group,
// guarded by the auth middleware. Effective paths: /api/v1/uploads/...
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, authMW gin.HandlerFunc) {
	g := rg.Group("/uploads", authMW)
	g.POST("", h.Create)
	g.GET("/:id", h.Get)
}
