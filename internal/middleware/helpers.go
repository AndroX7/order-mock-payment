package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/claudiovaldi/order-mock-payment/internal/httpresp"
)

func RequireUserID(c *gin.Context) (uuid.UUID, bool) {
	claims, ok := GetClaims(c)
	if !ok {
		httpresp.Error(c, http.StatusUnauthorized, httpresp.CodeUnauthorized, "authentication required")
		return uuid.Nil, false
	}
	return claims.UserID, true
}

func ParseIDParam(c *gin.Context, resource string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, httpresp.CodeInvalidRequest, "invalid "+resource+" id")
		return uuid.Nil, false
	}
	return id, true
}
