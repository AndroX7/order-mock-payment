// Package middleware provides Gin middlewares for cross-cutting concerns.
package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/claudiovaldi/order-mock-payment/internal/auth"
)

// TokenParser is the minimal parsing surface RequireAuth requires.
// Consumer-owned interface: middleware declares what it depends on;
// *auth.HMACTokenService satisfies it.
type TokenParser interface {
	Parse(token string) (auth.Claims, error)
}

// ClaimsContextKey is the gin.Context key under which validated Claims are stored.
const ClaimsContextKey = "auth_claims"

// CodeUnauthorized is the API error code returned when RequireAuth rejects a request.
const CodeUnauthorized = "UNAUTHORIZED"

var (
	errMissingHeader = errors.New("missing Authorization header")
	errInvalidScheme = errors.New("invalid Authorization scheme")
)

// RequireAuth returns a Gin middleware that validates a Bearer token
// and stores auth.Claims in the request context on success. Any failure
// aborts the request with 401.
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

// GetClaims retrieves the validated Claims from the request context.
// Callers must have run RequireAuth on the route.
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
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"success": false,
		"error": gin.H{
			"code":    CodeUnauthorized,
			"message": "authentication required",
		},
	})
}
