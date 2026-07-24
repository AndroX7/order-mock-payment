package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/claudiovaldi/order-mock-payment/internal/auth"
	"github.com/claudiovaldi/order-mock-payment/internal/httpresp"
)

type TokenParser interface {
	Parse(token string) (auth.Claims, error)
}

const ClaimsContextKey = "auth_claims"

var (
	errMissingHeader = errors.New("missing Authorization header")
	errInvalidScheme = errors.New("invalid Authorization scheme")
)

func RequireAuth(parser TokenParser, log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := extractBearer(c.GetHeader("Authorization"))
		if err != nil {
			unauthorized(c, log, err)
			return
		}
		claims, err := parser.Parse(token)
		if err != nil {
			unauthorized(c, log, err)
			return
		}
		c.Set(ClaimsContextKey, claims)
		c.Next()
	}
}

func GetClaims(c *gin.Context) (auth.Claims, bool) {
	v, ok := c.Get(ClaimsContextKey)
	if !ok {
		return auth.Claims{}, false
	}
	claims, ok := v.(auth.Claims)
	return claims, ok
}

func extractBearer(header string) (string, error) {
	if header == "" {
		return "", errMissingHeader
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", errInvalidScheme
	}
	return strings.TrimSpace(parts[1]), nil
}

func unauthorized(c *gin.Context, log *slog.Logger, err error) {
	log.Debug("auth rejected", "reason", err.Error(), "path", c.Request.URL.Path)
	c.Abort()
	httpresp.Error(c, http.StatusUnauthorized, httpresp.CodeUnauthorized, "authentication required")
}
