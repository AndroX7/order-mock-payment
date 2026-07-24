package order

import "errors"

var (
	ErrInvalidSymbol   = errors.New("symbol is required, uppercase, and at most 20 characters")
	ErrInvalidSide     = errors.New("side must be BUY or SELL")
	ErrInvalidQuantity = errors.New("quantity must be greater than 0")
	ErrInvalidPrice    = errors.New("price must be greater than or equal to 0")
	ErrOrderNotFound   = errors.New("order not found")
)
