package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/claudiovaldi/order-mock-payment/internal/httpresp"
)

// RequireUserID extracts the authenticated user's ID from JWT claims
// stored in context. On failure it writes a 401 response and returns
// ok=false; the caller should simply return.
//
// A properly wired route runs RequireAuth first, so ok=false is a
// defensive path against middleware misconfiguration.
func RequireUserID(c *gin.Context) (uuid.UUID, bool) {
	claims, ok := GetClaims(c)
	if !ok {
		httpresp.Error(c, http.StatusUnauthorized, httpresp.CodeUnauthorized, "authentication required")
		return uuid.Nil, false
	}
	return claims.UserID, true
}

// ParseIDParam parses c.Param("id") as a UUID. On failure writes a 400
// response with a resource-scoped message and returns ok=false.
func ParseIDParam(c *gin.Context, resource string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, httpresp.CodeInvalidRequest, "invalid "+resource+" id")
		return uuid.Nil, false
	}
	return id, true
}
