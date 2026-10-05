package payment

import "errors"

var (
	ErrOrderIDRequired     = errors.New("order id is required")
	ErrOrderIDTooLong      = errors.New("order id is too long")
	ErrInvalidAmount       = errors.New("amount must be greater than zero")
	ErrInvalidCurrency     = errors.New("invalid currency")
	ErrUnsupportedCurrency = errors.New("unsupported currency")
)
