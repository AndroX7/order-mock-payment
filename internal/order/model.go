package order

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	SideBuy  = "BUY"
	SideSell = "SELL"

	StatusPending        = "pending"
	StatusPendingPayment = "pending_payment"
	StatusPaid           = "paid"
	StatusPaymentFailed  = "payment_failed"
)

type Order struct {
	ID        uuid.UUID       `db:"id"`
	UserID    uuid.UUID       `db:"user_id"`
	Symbol    string          `db:"symbol"`
	Side      string          `db:"side"`
	Quantity  decimal.Decimal `db:"quantity"`
	Price     decimal.Decimal `db:"price"`
	Status    string          `db:"status"`
	CreatedAt time.Time       `db:"created_at"`
	UpdatedAt time.Time       `db:"updated_at"`
}
