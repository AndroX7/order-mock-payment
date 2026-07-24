package payment

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/claudiovaldi/order-mock-payment/internal/httpresp"
	"github.com/claudiovaldi/order-mock-payment/internal/middleware"
)

// Domain-specific API error codes. Cross-cutting codes
// (INVALID_REQUEST, INVALID_CONTENT_TYPE, UNAUTHORIZED, INTERNAL_ERROR)
// live in httpresp.
const (
	CodeOrderNotFound    = "ORDER_NOT_FOUND"
	CodeOrderNotPayable  = "ORDER_NOT_PAYABLE"
	CodeDuplicatePayment = "DUPLICATE_PAYMENT"
	CodeInvalidAmount    = "INVALID_AMOUNT"
	CodePaymentNotFound  = "PAYMENT_NOT_FOUND"
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
	userID, ok := middleware.RequireUserID(c)
	if !ok {
		return
	}
	if !httpresp.IsJSONContentType(c) {
		httpresp.Error(c, http.StatusUnsupportedMediaType, httpresp.CodeInvalidContentType,
			"Content-Type must be application/json")
		return
	}
	var req CreatePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, httpresp.CodeInvalidRequest, "malformed request body")
		return
	}
	if req.OrderID == uuid.Nil {
		httpresp.Error(c, http.StatusBadRequest, httpresp.CodeInvalidRequest, "order_id is required")
		return
	}
	p, err := h.svc.Create(c.Request.Context(), userID, req.OrderID)
	if err != nil {
		status, code, message := mapDomainError(err)
		httpresp.Error(c, status, code, message)
		return
	}
	httpresp.Success(c, http.StatusCreated, gin.H{"payment": NewPaymentResponse(p)})
}

// Get handles GET /api/v1/payments/:id.
func (h *Handler) Get(c *gin.Context) {
	userID, ok := middleware.RequireUserID(c)
	if !ok {
		return
	}
	paymentID, ok := middleware.ParseIDParam(c, "payment")
	if !ok {
		return
	}
	p, err := h.svc.Get(c.Request.Context(), userID, paymentID)
	if err != nil {
		status, code, message := mapDomainError(err)
		httpresp.Error(c, status, code, message)
		return
	}
	httpresp.Success(c, http.StatusOK, gin.H{"payment": NewPaymentResponse(p)})
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
		return http.StatusInternalServerError, httpresp.CodeInternalError, "internal server error"
	}
}
