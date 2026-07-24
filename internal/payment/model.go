package payment

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	StatusPending = "pending"
	StatusPaid    = "paid"
	StatusFailed  = "failed"
)

type Payment struct {
	ID                uuid.UUID       `db:"id"`
	OrderID           uuid.UUID       `db:"order_id"`
	Provider          string          `db:"provider"`
	ProviderReference string          `db:"provider_reference"`
	Amount            decimal.Decimal `db:"amount"`
	Currency          string          `db:"currency"`
	Status            string          `db:"status"`
	CreatedAt         time.Time       `db:"created_at"`
	UpdatedAt         time.Time       `db:"updated_at"`
}
