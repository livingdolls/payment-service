package payment

import "time"

type Status string

const (
	StatusCreated           Status = "CREATED"
	StatusProcessing        Status = "PROCESSING"
	StatusRequiresAction    Status = "REQUIRES_ACTION"
	StatusAuthorized        Status = "AUTHORIZED"
	StatusCaptured          Status = "CAPTURED"
	StatusPartiallyCaptured Status = "PARTIALLY_CAPTURED"
	StatusRefunded          Status = "REFUNDED"
	StatusFailed            Status = "FAILED"
	StatusCanceled          Status = "CANCELED"
	StatusExpired           Status = "EXPIRED"
)

type CaptureMethod string

const (
	CaptureMethodAutomatic CaptureMethod = "AUTOMATIC"
	CaptureMethodManual    CaptureMethod = "MANUAL"
)

type PaymentIntent struct {
	ID string

	ReferenceID string
	OrderID     string

	Amount   int64
	Currency string

	Status        Status
	CaptureMethod CaptureMethod

	CapturedAmount int64
	RefundedAmount int64

	Version int64

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (p *PaymentIntent) TransitionTo(next Status) error {
	if p.Status == next {
		return nil
	}

	if !canTransitionPayment(p.Status, next) {
		return ErrInvalidPaymentTransition
	}

	p.Status = next

	return nil
}

func canTransitionPayment(current Status, next Status) bool {
	switch current {
	case StatusCreated:
		switch next {
		case StatusProcessing, StatusFailed, StatusCanceled:

			return true
		}

	case StatusProcessing:
		switch next {
		case StatusRequiresAction, StatusAuthorized, StatusCaptured, StatusFailed, StatusCanceled, StatusExpired:

			return true
		}

	case StatusRequiresAction:
		switch next {
		case StatusProcessing, StatusAuthorized, StatusCaptured, StatusFailed, StatusCanceled, StatusExpired:

			return true
		}

	case StatusAuthorized:
		switch next {
		case StatusCaptured, StatusFailed, StatusCanceled:

			return true
		}

	case StatusCaptured:
		switch next {
		case StatusPartiallyCaptured, StatusRefunded:

			return true
		}

	case StatusRefunded:
		return next == StatusRefunded
	}

	return false
}
