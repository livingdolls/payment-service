package provider

import "context"

type PaymentStatus string

const (
	PaymentStatusPending        PaymentStatus = "PENDING"
	PaymentStatusRequiresAction PaymentStatus = "REQUIRES_ACTION"
	PaymentStatusAuthorized     PaymentStatus = "AUTHORIZED"
	PaymentStatusSucceeded      PaymentStatus = "SUCCEEDED"
	PaymentStatusFail           PaymentStatus = "FAILED"
	PaymentStatusExpired        PaymentStatus = "EXPIRED"
	PaymentStatusCanceled       PaymentStatus = "CANCELED"
	PaymentStatusUnknown        PaymentStatus = "UNKNOWN"
)

type Action struct {
	Type       string `json:"type"`
	Descriptor string `json:"descriptor"`
	Value      string `json:"value"`
}

type CreatePaymentInput struct {
	ReferenceID string

	Amount   int64
	Currency string

	CaptureMethod     string
	Country           string
	ChannelCode       string
	ChannelProperties map[string]any
	Description       string
	Metadata          map[string]any
}

type CreatePaymentResult struct {
	PaymentRequestID string
	PaymentID        *string
	Status           PaymentStatus
	Actions          []Action
	RawResponse      []byte
}

type PaymentRequestSnapshot struct {
	PaymentRequestID string
	ReferenceID      string

	Amount   int64
	Currency string

	PaymentID *string
	Status    PaymentStatus

	Actions     []Action
	RawResponse []byte
}

type PaymentProvider interface {
	CreatePayment(ctx context.Context, input CreatePaymentInput) (*CreatePaymentResult, error)
}

type PaymentStatusReader interface {
	GetPaymentRequest(ctx context.Context, paymentRequestID string) (*PaymentRequestSnapshot, error)
}

type TransactionReferenceLookup interface {
	FindPaymentRequestIDByReference(ctx context.Context, referenceID string, currency string, amount int64) (string, bool, error)
}
