package order

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
	CodeInvalidSymbol   = "INVALID_SYMBOL"
	CodeInvalidSide     = "INVALID_SIDE"
	CodeInvalidQuantity = "INVALID_QUANTITY"
	CodeInvalidPrice    = "INVALID_PRICE"
	CodeOrderNotFound   = "ORDER_NOT_FOUND"
)

// OrderService is the minimal service surface Handler requires.
// Consumer-owned interface; concrete *Service satisfies it.
type OrderService interface {
	Create(ctx context.Context, userID uuid.UUID, req CreateOrderRequest) (*Order, error)
	Get(ctx context.Context, userID, orderID uuid.UUID) (*Order, error)
	List(ctx context.Context, userID uuid.UUID) ([]*Order, error)
	Update(ctx context.Context, userID, orderID uuid.UUID, req UpdateOrderRequest) (*Order, error)
	Delete(ctx context.Context, userID, orderID uuid.UUID) error
}

// Handler serves HTTP requests for the order resource.
type Handler struct {
	svc OrderService
}

func NewHandler(svc OrderService) *Handler {
	return &Handler{svc: svc}
}

// Create handles POST /api/v1/orders.
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
	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, httpresp.CodeInvalidRequest, "malformed request body")
		return
	}
	order, err := h.svc.Create(c.Request.Context(), userID, req)
	if err != nil {
		status, code, message := mapDomainError(err)
		httpresp.Error(c, status, code, message)
		return
	}
	httpresp.Success(c, http.StatusCreated, gin.H{"order": NewOrderResponse(order)})
}

// Get handles GET /api/v1/orders/:id.
func (h *Handler) Get(c *gin.Context) {
	userID, ok := middleware.RequireUserID(c)
	if !ok {
		return
	}
	orderID, ok := middleware.ParseIDParam(c, "order")
	if !ok {
		return
	}
	order, err := h.svc.Get(c.Request.Context(), userID, orderID)
	if err != nil {
		status, code, message := mapDomainError(err)
		httpresp.Error(c, status, code, message)
		return
	}
	httpresp.Success(c, http.StatusOK, gin.H{"order": NewOrderResponse(order)})
}

// List handles GET /api/v1/orders.
func (h *Handler) List(c *gin.Context) {
	userID, ok := middleware.RequireUserID(c)
	if !ok {
		return
	}
	orders, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		status, code, message := mapDomainError(err)
		httpresp.Error(c, status, code, message)
		return
	}
	out := make([]OrderResponse, len(orders))
	for i, o := range orders {
		out[i] = NewOrderResponse(o)
	}
	httpresp.Success(c, http.StatusOK, gin.H{"orders": out})
}

// Update handles PUT /api/v1/orders/:id.
func (h *Handler) Update(c *gin.Context) {
	userID, ok := middleware.RequireUserID(c)
	if !ok {
		return
	}
	orderID, ok := middleware.ParseIDParam(c, "order")
	if !ok {
		return
	}
	if !httpresp.IsJSONContentType(c) {
		httpresp.Error(c, http.StatusUnsupportedMediaType, httpresp.CodeInvalidContentType,
			"Content-Type must be application/json")
		return
	}
	var req UpdateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, httpresp.CodeInvalidRequest, "malformed request body")
		return
	}
	order, err := h.svc.Update(c.Request.Context(), userID, orderID, req)
	if err != nil {
		status, code, message := mapDomainError(err)
		httpresp.Error(c, status, code, message)
		return
	}
	httpresp.Success(c, http.StatusOK, gin.H{"order": NewOrderResponse(order)})
}

// Delete handles DELETE /api/v1/orders/:id.
func (h *Handler) Delete(c *gin.Context) {
	userID, ok := middleware.RequireUserID(c)
	if !ok {
		return
	}
	orderID, ok := middleware.ParseIDParam(c, "order")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), userID, orderID); err != nil {
		status, code, message := mapDomainError(err)
		httpresp.Error(c, status, code, message)
		return
	}
	c.Status(http.StatusNoContent)
}

// mapDomainError converts a service/repo error into (HTTP status, API code, safe message).
func mapDomainError(err error) (int, string, string) {
	switch {
	case errors.Is(err, ErrInvalidSymbol):
		return http.StatusBadRequest, CodeInvalidSymbol, err.Error()
	case errors.Is(err, ErrInvalidSide):
		return http.StatusBadRequest, CodeInvalidSide, err.Error()
	case errors.Is(err, ErrInvalidQuantity):
		return http.StatusBadRequest, CodeInvalidQuantity, err.Error()
	case errors.Is(err, ErrInvalidPrice):
		return http.StatusBadRequest, CodeInvalidPrice, err.Error()
	case errors.Is(err, ErrOrderNotFound):
		return http.StatusNotFound, CodeOrderNotFound, err.Error()
	default:
		return http.StatusInternalServerError, httpresp.CodeInternalError, "internal server error"
	}
}
