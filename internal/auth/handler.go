package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// API error codes returned in the response envelope. Handler-layer
// constants — the domain (service/repo) does not know about them.
const (
	CodeInvalidRequest     = "INVALID_REQUEST"
	CodeInvalidContentType = "INVALID_CONTENT_TYPE"
	CodeInvalidEmail       = "INVALID_EMAIL"
	CodeInvalidName        = "INVALID_NAME"
	CodePasswordTooShort   = "PASSWORD_TOO_SHORT"
	CodePasswordTooLong    = "PASSWORD_TOO_LONG"
	CodeEmailAlreadyExists = "EMAIL_ALREADY_EXISTS"
	CodeInvalidCredentials = "INVALID_CREDENTIALS"
	CodeInternalError      = "INTERNAL_ERROR"
)

// AuthService is the minimal service surface the Handler needs.
// Consumer-owned interface: the handler declares what it requires; the
// concrete *Service (or any test double) satisfies it.
type AuthService interface {
	Signup(ctx context.Context, req SignupRequest) (*User, error)
	Login(ctx context.Context, req LoginRequest) (*User, string, error)
}

// Handler serves HTTP requests for the auth resource.
type Handler struct {
	svc AuthService
}

func NewHandler(svc AuthService) *Handler {
	return &Handler{svc: svc}
}

// Signup handles POST /api/v1/auth/signup.
func (h *Handler) Signup(c *gin.Context) {
	if !isJSONContentType(c) {
		respondError(c, http.StatusUnsupportedMediaType, CodeInvalidContentType,
			"Content-Type must be application/json")
		return
	}

	var req SignupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest,
			"malformed request body")
		return
	}

	user, err := h.svc.Signup(c.Request.Context(), req)
	if err != nil {
		status, code, message := mapDomainError(err)
		respondError(c, status, code, message)
		return
	}

	respondSuccess(c, http.StatusCreated, gin.H{
		"user": NewUserResponse(user),
	})
}

// Login handles POST /api/v1/auth/login.
func (h *Handler) Login(c *gin.Context) {
	if !isJSONContentType(c) {
		respondError(c, http.StatusUnsupportedMediaType, CodeInvalidContentType,
			"Content-Type must be application/json")
		return
	}

	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "malformed request body")
		return
	}

	user, token, err := h.svc.Login(c.Request.Context(), req)
	if err != nil {
		status, code, message := mapDomainError(err)
		respondError(c, status, code, message)
		return
	}

	respondSuccess(c, http.StatusOK, gin.H{
		"token": token,
		"user":  NewUserResponse(user),
	})
}

func isJSONContentType(c *gin.Context) bool {
	// c.ContentType() strips parameters like "; charset=utf-8".
	return strings.EqualFold(c.ContentType(), "application/json")
}

// mapDomainError maps a service/repo error into (HTTP status, API code, safe message).
// Unknown errors are collapsed to 500 + generic message so no internal detail leaks.
func mapDomainError(err error) (int, string, string) {
	switch {
	case errors.Is(err, ErrInvalidEmail):
		return http.StatusBadRequest, CodeInvalidEmail, err.Error()
	case errors.Is(err, ErrEmptyName):
		return http.StatusBadRequest, CodeInvalidName, err.Error()
	case errors.Is(err, ErrPasswordTooShort):
		return http.StatusBadRequest, CodePasswordTooShort, err.Error()
	case errors.Is(err, ErrPasswordTooLong):
		return http.StatusBadRequest, CodePasswordTooLong, err.Error()
	case errors.Is(err, ErrEmailAlreadyExists):
		return http.StatusConflict, CodeEmailAlreadyExists, err.Error()
	case errors.Is(err, ErrInvalidCredentials):
		return http.StatusUnauthorized, CodeInvalidCredentials, "invalid email or password"
	default:
		return http.StatusInternalServerError, CodeInternalError, "internal server error"
	}
}

func respondSuccess(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{
		"success": true,
		"data":    data,
	})
}

func respondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{
		"success": false,
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}
