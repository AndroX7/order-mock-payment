package order

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// CreateOrderRequest is the JSON payload for POST /api/v1/orders.
// user_id is never accepted from clients — it comes from JWT claims.
type CreateOrderRequest struct {
	Symbol   string          `json:"symbol"`
	Side     string          `json:"side"`
	Quantity decimal.Decimal `json:"quantity"`
	Price    decimal.Decimal `json:"price"`
}

// UpdateOrderRequest is the JSON payload for PUT /api/v1/orders/:id.
// Full replace semantics: all fields required. status is server-controlled
// and intentionally absent.
type UpdateOrderRequest struct {
	Symbol   string          `json:"symbol"`
	Side     string          `json:"side"`
	Quantity decimal.Decimal `json:"quantity"`
	Price    decimal.Decimal `json:"price"`
}

// OrderResponse is the client-facing view of an Order.
type OrderResponse struct {
	ID        uuid.UUID       `json:"id"`
	UserID    uuid.UUID       `json:"user_id"`
	Symbol    string          `json:"symbol"`
	Side      string          `json:"side"`
	Quantity  decimal.Decimal `json:"quantity"`
	Price     decimal.Decimal `json:"price"`
	Status    string          `json:"status"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// NewOrderResponse builds the DTO from a domain Order.
func NewOrderResponse(o *Order) OrderResponse {
	return OrderResponse{
		ID:        o.ID,
		UserID:    o.UserID,
		Symbol:    o.Symbol,
		Side:      o.Side,
		Quantity:  o.Quantity,
		Price:     o.Price,
		Status:    o.Status,
		CreatedAt: o.CreatedAt,
		UpdatedAt: o.UpdatedAt,
	}
}
