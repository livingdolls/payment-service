package order

import "errors"

var (
	ErrInvalidEvent              = errors.New("invalid payment event")
	ErrOrderNotFound             = errors.New("order not found")
	ErrPaymentNotBound           = errors.New("order has no bound payment intent")
	ErrPaymentMismatch           = errors.New("payment does not match order")
	ErrOrderNotPayable           = errors.New("order cannot be marked as paid")
	ErrEventPayloadConflict      = errors.New("event id already exists with different payload")
	ErrCheckoutPaymentMismatch   = errors.New("checkout payment does not match order")
	ErrCheckoutAlreadyBound      = errors.New("order is bound to another payment intent")
	ErrCheckoutOrderChanged      = errors.New("order changed during checkout")
	ErrPaymentServiceUnavailable = errors.New("payment service request failed")
)
