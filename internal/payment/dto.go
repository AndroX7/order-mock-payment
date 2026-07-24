package payment

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type CreatePaymentRequest struct {
	OrderID uuid.UUID `json:"order_id"`
}

type PaymentResponse struct {
	ID                uuid.UUID       `json:"id"`
	OrderID           uuid.UUID       `json:"order_id"`
	Provider          string          `json:"provider"`
	ProviderReference string          `json:"provider_reference"`
	Amount            decimal.Decimal `json:"amount"`
	Currency          string          `json:"currency"`
	Status            string          `json:"status"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

func NewPaymentResponse(p *Payment) PaymentResponse {
	return PaymentResponse{
		ID:                p.ID,
		OrderID:           p.OrderID,
		Provider:          p.Provider,
		ProviderReference: p.ProviderReference,
		Amount:            p.Amount,
		Currency:          p.Currency,
		Status:            p.Status,
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
	}
}
