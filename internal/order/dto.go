package order

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type CreateOrderRequest struct {
	Symbol   string          `json:"symbol"`
	Side     string          `json:"side"`
	Quantity decimal.Decimal `json:"quantity"`
	Price    decimal.Decimal `json:"price"`
}

type UpdateOrderRequest struct {
	Symbol   string          `json:"symbol"`
	Side     string          `json:"side"`
	Quantity decimal.Decimal `json:"quantity"`
	Price    decimal.Decimal `json:"price"`
}

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
