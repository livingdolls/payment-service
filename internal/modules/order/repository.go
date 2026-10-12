package order

import (
	"context"

	"github.com/livingdolls/payment-service/internal/modules/outbox"
)

type Repository interface {
	ConsumePaymentCaptured(
		ctx context.Context,
		event outbox.PaymentCapturedPayload,
		payloadHash string,
	) (duplicate bool, err error)
}
