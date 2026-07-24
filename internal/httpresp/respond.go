// Package httpresp holds the standard JSON response envelope and the
// cross-cutting API error codes shared by every handler.
//
// It is deliberately minimal — two response functions, one Content-Type
// check, four constants. Domain-specific codes stay in their owning
// package.
package httpresp

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// Cross-cutting API error codes. Domain-specific codes stay in their
// respective handler packages.
const (
	CodeInvalidRequest     = "INVALID_REQUEST"
	CodeInvalidContentType = "INVALID_CONTENT_TYPE"
	CodeInternalError      = "INTERNAL_ERROR"
	CodeUnauthorized       = "UNAUTHORIZED"
)

// Success writes {"success": true, "data": data} with the given status.
func Success(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{
		"success": true,
		"data":    data,
	})
}

// Error writes {"success": false, "error": {"code": code, "message": message}}
// with the given status. Callers decide whether to also call c.Abort().
func Error(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{
		"success": false,
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}

// IsJSONContentType reports whether the request's Content-Type is
// application/json. Parameters like "; charset=utf-8" are stripped
// by gin's ContentType().
func IsJSONContentType(c *gin.Context) bool {
	return strings.EqualFold(c.ContentType(), "application/json")
}
