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
