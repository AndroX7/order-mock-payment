package webhook

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the webhook handler on the given router group.
// No auth middleware — the provider signature (verified per request)
// is the authorization mechanism.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/payment", h.Callback)
}
