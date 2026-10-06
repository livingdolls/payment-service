package payment

import "time"

type AttemptStatus string

const (
	AttemptStatusCreated            AttemptStatus = "CREATED"
	AttemptStatusRequestingProvider AttemptStatus = "REQUESTING_PROVIDER"
	AttemptStatusRequiresAction     AttemptStatus = "REQUIRES_ACTION"
	AttemptStatusAuthorized         AttemptStatus = "AUTHORIZED"
	AttemptStatusCaptured           AttemptStatus = "CAPTURED"
	AttemptStatusFailed             AttemptStatus = "FAILED"
	AttemptStatusExpired            AttemptStatus = "EXPIRED"
	AttemptStatusUnknown            AttemptStatus = "UNKNOWN"
)

type Provider string

const (
	ProviderXendit Provider = "XENDIT"
)

type PaymentAttempt struct {
	ID string

	PaymentIntentID string

	AttemptNumber int

	Provider Provider

	ProviderIdempotencyKey string

	ProviderPaymentRequestID *string
	ProviderPaymentID        *string

	Status AttemptStatus

	ErrorCode    *string
	ErrorMessage *string

	ProviderResponse []byte

	CreatedAt time.Time
	UpdatedAt time.Time
}
