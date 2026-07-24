package payment

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/claudiovaldi/order-mock-payment/internal/middleware"
)

// API error codes returned in the response envelope.
const (
	CodeInvalidRequest     = "INVALID_REQUEST"
	CodeInvalidContentType = "INVALID_CONTENT_TYPE"
	CodeOrderNotFound      = "ORDER_NOT_FOUND"
	CodeOrderNotPayable    = "ORDER_NOT_PAYABLE"
	CodeDuplicatePayment   = "DUPLICATE_PAYMENT"
	CodeInvalidAmount      = "INVALID_AMOUNT"
	CodePaymentNotFound    = "PAYMENT_NOT_FOUND"
	CodeUnauthorized       = "UNAUTHORIZED"
	CodeInternalError      = "INTERNAL_ERROR"
)

// PaymentService is the minimal service surface the Handler requires.
type PaymentService interface {
	Create(ctx context.Context, userID, orderID uuid.UUID) (*Payment, error)
	Get(ctx context.Context, userID, paymentID uuid.UUID) (*Payment, error)
}

type Handler struct {
	svc PaymentService
}

func NewHandler(svc PaymentService) *Handler {
	return &Handler{svc: svc}
}

// Create handles POST /api/v1/payments.
func (h *Handler) Create(c *gin.Context) {
	userID, ok := requireUser(c)
	if !ok {
		return
	}
	if !isJSONContentType(c) {
		respondError(c, http.StatusUnsupportedMediaType, CodeInvalidContentType,
			"Content-Type must be application/json")
		return
	}
	var req CreatePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "malformed request body")
		return
	}
	if req.OrderID == uuid.Nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "order_id is required")
		return
	}
	p, err := h.svc.Create(c.Request.Context(), userID, req.OrderID)
	if err != nil {
		status, code, message := mapDomainError(err)
		respondError(c, status, code, message)
		return
	}
	respondSuccess(c, http.StatusCreated, gin.H{"payment": NewPaymentResponse(p)})
}

// Get handles GET /api/v1/payments/:id.
func (h *Handler) Get(c *gin.Context) {
	userID, ok := requireUser(c)
	if !ok {
		return
	}
	paymentID, ok := parseIDParam(c)
	if !ok {
		return
	}
	p, err := h.svc.Get(c.Request.Context(), userID, paymentID)
	if err != nil {
		status, code, message := mapDomainError(err)
		respondError(c, status, code, message)
		return
	}
	respondSuccess(c, http.StatusOK, gin.H{"payment": NewPaymentResponse(p)})
}

// --- helpers ---

func requireUser(c *gin.Context) (uuid.UUID, bool) {
	claims, ok := middleware.GetClaims(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, CodeUnauthorized, "authentication required")
		return uuid.Nil, false
	}
	return claims.UserID, true
}

func parseIDParam(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "invalid payment id")
		return uuid.Nil, false
	}
	return id, true
}

func isJSONContentType(c *gin.Context) bool {
	return strings.EqualFold(c.ContentType(), "application/json")
}

func mapDomainError(err error) (int, string, string) {
	switch {
	case errors.Is(err, ErrOrderNotFound):
		return http.StatusNotFound, CodeOrderNotFound, err.Error()
	case errors.Is(err, ErrOrderNotPayable):
		return http.StatusConflict, CodeOrderNotPayable, err.Error()
	case errors.Is(err, ErrDuplicatePayment):
		return http.StatusConflict, CodeDuplicatePayment, err.Error()
	case errors.Is(err, ErrInvalidAmount):
		return http.StatusBadRequest, CodeInvalidAmount, err.Error()
	case errors.Is(err, ErrPaymentNotFound):
		return http.StatusNotFound, CodePaymentNotFound, err.Error()
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
