package xendit

import "time"

type PaymentWebhook struct {
	Event      string    `json:"event"`
	BusinessID string    `json:"business_id"`
	Created    time.Time `json:"created"`

	Data PaymentWebhookData `json:"data"`
}

type PaymentWebhookData struct {
	PaymentID        string                  `json:"payment_id"`
	PaymentRequestID string                  `json:"payment_request_id"`
	ReferenceID      string                  `json:"reference_id"`
	Status           string                  `json:"status"`
	FailureCode      string                  `json:"failure_code,omitempty"`
	Captures         []PaymentWebhookCapture `json:"captures,omitempty"`
}

type PaymentWebhookCapture struct {
	CaptureID        string    `json:"capture_id"`
	CaptureTimestamp time.Time `json:"capture_timestamp"`
	CaptureAmount    int64     `json:"capture_amount"`
}

func PaymentWebhookEventKey(event PaymentWebhook) string {
	if event.Event == "payment.capture" && len(event.Data.Captures) > 0 && event.Data.Captures[0].CaptureID != "" {

		return event.Event + ":" + event.Data.Captures[0].CaptureID
	}

	return event.Event + ":" + event.Data.PaymentID + ":" + event.Created.UTC().Format(time.RFC3339Nano)
}
