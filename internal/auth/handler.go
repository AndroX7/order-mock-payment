package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/claudiovaldi/order-mock-payment/internal/httpresp"
)

// Domain-specific API error codes returned by this handler.
// Cross-cutting codes (INVALID_REQUEST, INVALID_CONTENT_TYPE, INTERNAL_ERROR)
// live in httpresp.
const (
	CodeInvalidEmail       = "INVALID_EMAIL"
	CodeInvalidName        = "INVALID_NAME"
	CodePasswordTooShort   = "PASSWORD_TOO_SHORT"
	CodePasswordTooLong    = "PASSWORD_TOO_LONG"
	CodeEmailAlreadyExists = "EMAIL_ALREADY_EXISTS"
	CodeInvalidCredentials = "INVALID_CREDENTIALS"
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
	if !httpresp.IsJSONContentType(c) {
		httpresp.Error(c, http.StatusUnsupportedMediaType, httpresp.CodeInvalidContentType,
			"Content-Type must be application/json")
		return
	}

	var req SignupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, httpresp.CodeInvalidRequest,
			"malformed request body")
		return
	}

	user, err := h.svc.Signup(c.Request.Context(), req)
	if err != nil {
		status, code, message := mapDomainError(err)
		httpresp.Error(c, status, code, message)
		return
	}

	httpresp.Success(c, http.StatusCreated, gin.H{
		"user": NewUserResponse(user),
	})
}

// Login handles POST /api/v1/auth/login.
func (h *Handler) Login(c *gin.Context) {
	if !httpresp.IsJSONContentType(c) {
		httpresp.Error(c, http.StatusUnsupportedMediaType, httpresp.CodeInvalidContentType,
			"Content-Type must be application/json")
		return
	}

	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, httpresp.CodeInvalidRequest, "malformed request body")
		return
	}

	user, token, err := h.svc.Login(c.Request.Context(), req)
	if err != nil {
		status, code, message := mapDomainError(err)
		httpresp.Error(c, status, code, message)
		return
	}

	httpresp.Success(c, http.StatusOK, gin.H{
		"token": token,
		"user":  NewUserResponse(user),
	})
}

// mapDomainError maps a service/repo error into (HTTP status, API code, safe message).
// Unknown errors collapse to 500 + generic message so no internal detail leaks.
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
		return http.StatusInternalServerError, httpresp.CodeInternalError, "internal server error"
	}
}
