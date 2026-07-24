package payment

import "errors"

var (
	ErrOrderNotFound           = errors.New("order not found")
	ErrOrderNotPayable         = errors.New("order is not in a payable state")
	ErrDuplicatePayment        = errors.New("payment already exists for this order")
	ErrInvalidAmount           = errors.New("payment amount must be greater than 0")
	ErrPaymentNotFound         = errors.New("payment not found")
	ErrInvalidStatusTransition = errors.New("invalid payment status transition")
)
