package xendit

type createPaymentRequest struct {
	ReferenceID string `json:"reference_id"`

	Type string `json:"type"`

	Country  string `json:"country"`
	Currency string `json:"currency"`

	RequestAmount int64 `json:"request_amount"`

	CaptureMethod string `json:"capture_method"`

	ChannelCode string `json:"channel_code"`

	ChannelProperties map[string]any `json:"channel_properties,omitempty"`

	Description string `json:"description,omitempty"`

	Metadata map[string]any `json:"metadata,omitempty"`
}

type createPaymentResponse struct {
	PaymentRequestID string `json:"payment_request_id"`

	ReferenceID string `json:"reference_id"`

	Status string `json:"status"`

	LatestPaymentID *string `json:"latest_payment_id"`

	Actions []struct {
		Type       string `json:"type"`
		Descriptor string `json:"descriptor"`
		Value      string `json:"value"`
	} `json:"actions"`
}

type errorResponse struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}
