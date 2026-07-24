package order

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
	CodeInvalidSymbol      = "INVALID_SYMBOL"
	CodeInvalidSide        = "INVALID_SIDE"
	CodeInvalidQuantity    = "INVALID_QUANTITY"
	CodeInvalidPrice       = "INVALID_PRICE"
	CodeOrderNotFound      = "ORDER_NOT_FOUND"
	CodeUnauthorized       = "UNAUTHORIZED"
	CodeInternalError      = "INTERNAL_ERROR"
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
	userID, ok := requireUser(c)
	if !ok {
		return
	}
	if !isJSONContentType(c) {
		respondError(c, http.StatusUnsupportedMediaType, CodeInvalidContentType,
			"Content-Type must be application/json")
		return
	}
	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "malformed request body")
		return
	}
	order, err := h.svc.Create(c.Request.Context(), userID, req)
	if err != nil {
		status, code, message := mapDomainError(err)
		respondError(c, status, code, message)
		return
	}
	respondSuccess(c, http.StatusCreated, gin.H{"order": NewOrderResponse(order)})
}

// Get handles GET /api/v1/orders/:id.
func (h *Handler) Get(c *gin.Context) {
	userID, ok := requireUser(c)
	if !ok {
		return
	}
	orderID, ok := parseIDParam(c)
	if !ok {
		return
	}
	order, err := h.svc.Get(c.Request.Context(), userID, orderID)
	if err != nil {
		status, code, message := mapDomainError(err)
		respondError(c, status, code, message)
		return
	}
	respondSuccess(c, http.StatusOK, gin.H{"order": NewOrderResponse(order)})
}

// List handles GET /api/v1/orders.
func (h *Handler) List(c *gin.Context) {
	userID, ok := requireUser(c)
	if !ok {
		return
	}
	orders, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		status, code, message := mapDomainError(err)
		respondError(c, status, code, message)
		return
	}
	out := make([]OrderResponse, len(orders))
	for i, o := range orders {
		out[i] = NewOrderResponse(o)
	}
	respondSuccess(c, http.StatusOK, gin.H{"orders": out})
}

// Update handles PUT /api/v1/orders/:id.
func (h *Handler) Update(c *gin.Context) {
	userID, ok := requireUser(c)
	if !ok {
		return
	}
	orderID, ok := parseIDParam(c)
	if !ok {
		return
	}
	if !isJSONContentType(c) {
		respondError(c, http.StatusUnsupportedMediaType, CodeInvalidContentType,
			"Content-Type must be application/json")
		return
	}
	var req UpdateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "malformed request body")
		return
	}
	order, err := h.svc.Update(c.Request.Context(), userID, orderID, req)
	if err != nil {
		status, code, message := mapDomainError(err)
		respondError(c, status, code, message)
		return
	}
	respondSuccess(c, http.StatusOK, gin.H{"order": NewOrderResponse(order)})
}

// Delete handles DELETE /api/v1/orders/:id.
func (h *Handler) Delete(c *gin.Context) {
	userID, ok := requireUser(c)
	if !ok {
		return
	}
	orderID, ok := parseIDParam(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), userID, orderID); err != nil {
		status, code, message := mapDomainError(err)
		respondError(c, status, code, message)
		return
	}
	c.Status(http.StatusNoContent)
}

// --- helpers ---

// requireUser extracts the authenticated user's ID from JWT claims stored
// in context. Guards against the middleware being misconfigured — should
// never fail on a properly protected route.
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
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "invalid order id")
		return uuid.Nil, false
	}
	return id, true
}

func isJSONContentType(c *gin.Context) bool {
	return strings.EqualFold(c.ContentType(), "application/json")
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
