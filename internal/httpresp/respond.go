package httpresp

import (
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	CodeInvalidRequest     = "INVALID_REQUEST"
	CodeInvalidContentType = "INVALID_CONTENT_TYPE"
	CodeInternalError      = "INTERNAL_ERROR"
	CodeUnauthorized       = "UNAUTHORIZED"
)

func Success(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{
		"success": true,
		"data":    data,
	})
}

func Error(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{
		"success": false,
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}

func IsJSONContentType(c *gin.Context) bool {
	return strings.EqualFold(c.ContentType(), "application/json")
}
