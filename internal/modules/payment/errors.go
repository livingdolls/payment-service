package payment

import "errors"

var (
	ErrOrderIDRequired          = errors.New("order id is required")
	ErrOrderIDTooLong           = errors.New("order id is too long")
	ErrInvalidAmount            = errors.New("amount must be greater than zero")
	ErrInvalidCurrency          = errors.New("invalid currency")
	ErrUnsupportedCurrency      = errors.New("unsupported currency")
	ErrInvalidAttemptTransition = errors.New("invalid payment attempt state transition")
	ErrAttemptConcurrentUpdate  = errors.New("payment attempt was modified concurrently")
	ErrInvalidPaymentTransition = errors.New("invalid payment state transition")
	ErrPaymentConcurrentUpdate  = errors.New("payment was modifed concurrently")
)
