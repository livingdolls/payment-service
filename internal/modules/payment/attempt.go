package payment

import "time"

type AttemptStatus string

const (
	AttemptStatusCreated            AttemptStatus = "CREATED"
	AttemptStatusRequestingProvider AttemptStatus = "REQUESTING_PROVIDER"
	AttemptStatusPending            AttemptStatus = "PENDING"
	AttemptStatusRequiresAction     AttemptStatus = "REQUIRES_ACTION"
	AttemptStatusAuthorized         AttemptStatus = "AUTHORIZED"
	AttemptStatusCaptured           AttemptStatus = "CAPTURED"
	AttemptStatusFailed             AttemptStatus = "FAILED"
	AttemptStatusExpired            AttemptStatus = "EXPIRED"
	AttemptStatusCanceled           AttemptStatus = "CANCELED"
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

	ProviderRequest  []byte
	ProviderResponse []byte

	Version int64

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (a *PaymentAttempt) TransitionTo(next AttemptStatus) error {
	if a.Status == next {
		return nil
	}

	if !canTransitionAttempt(a.Status, next) {
		return ErrInvalidAttemptTransition
	}

	a.Status = next

	return nil
}

func canTransitionAttempt(
	current AttemptStatus,
	next AttemptStatus,
) bool {
	switch current {

	case AttemptStatusCreated:
		return next == AttemptStatusRequestingProvider

	case AttemptStatusRequestingProvider:
		switch next {
		case AttemptStatusPending,
			AttemptStatusRequiresAction,
			AttemptStatusAuthorized,
			AttemptStatusCaptured,
			AttemptStatusFailed,
			AttemptStatusExpired,
			AttemptStatusCanceled,
			AttemptStatusUnknown:

			return true
		}

	case AttemptStatusPending:
		switch next {
		case AttemptStatusRequiresAction,
			AttemptStatusAuthorized,
			AttemptStatusCaptured,
			AttemptStatusFailed,
			AttemptStatusExpired,
			AttemptStatusCanceled,
			AttemptStatusUnknown:

			return true
		}

	case AttemptStatusRequiresAction:
		switch next {
		case AttemptStatusAuthorized,
			AttemptStatusCaptured,
			AttemptStatusFailed,
			AttemptStatusExpired,
			AttemptStatusCanceled,
			AttemptStatusUnknown:

			return true
		}

	case AttemptStatusAuthorized:
		switch next {
		case AttemptStatusCaptured,
			AttemptStatusFailed,
			AttemptStatusCanceled,
			AttemptStatusUnknown:

			return true
		}

	case AttemptStatusUnknown:
		switch next {
		case AttemptStatusRequestingProvider,
			AttemptStatusPending,
			AttemptStatusRequiresAction,
			AttemptStatusAuthorized,
			AttemptStatusCaptured,
			AttemptStatusFailed,
			AttemptStatusExpired,
			AttemptStatusCanceled:

			return true
		}
	}

	return false
}
