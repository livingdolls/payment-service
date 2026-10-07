package webhook

import "time"

type Status string

const (
	StatusReceived   Status = "RECEIVED"
	StatusProcessing Status = "PROCESSING"
	StatusProcessed  Status = "PROCESSED"
	StatusFailed     Status = "FAILED"
)

type Event struct {
	ID string

	Provider string

	EventKey  string
	EventType string

	ProviderPaymentID        *string
	ProviderPaymentRequestID *string
	ReferenceID              *string

	Payload []byte

	Status Status

	ProcessingAttempts int

	ErrorMessage *string

	ReceivedAt          time.Time
	ProcessingStartedAt *time.Time
	ProcessedAt         *time.Time
}
