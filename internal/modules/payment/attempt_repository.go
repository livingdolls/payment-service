package payment

import "context"

type AttemptRepository interface {
	CreateAttempt(ctx context.Context, attempt *PaymentAttempt) error
	GetAttemptByID(ctx context.Context, id string) (*PaymentAttempt, error)
	GetLatestAttempt(ctx context.Context, paymentIntentId string) (*PaymentAttempt, error)
}
