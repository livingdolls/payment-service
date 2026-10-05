package payment

import "context"

type Repository interface {
	Create(
		ctx context.Context,
		payment *PaymentIntent,
	) error

	GetByID(
		ctx context.Context,
		id string,
	) (*PaymentIntent, error)
}
