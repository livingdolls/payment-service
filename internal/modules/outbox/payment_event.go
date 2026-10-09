package outbox

import "time"

const EventPaymentCaptured = "payment.captured.v1"

type PaymentCapturedPayload struct {
	EventID         string    `json:"event_id"`
	EventType       string    `json:"event_type"`
	PaymentIntentID string    `json:"payment_intent_id"`
	ReferenceID     string    `json:"reference_id"`
	OrderID         string    `json:"order_id"`
	Amount          int64     `json:"amount"`
	Currency        string    `json:"currency"`
	CapturedAmount  int64     `json:"captured_amount"`
	PaymentVersion  int64     `json:"payment_version"`
	OccurredAt      time.Time `json:"occurred_at"`
}
