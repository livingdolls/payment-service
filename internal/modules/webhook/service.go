package webhook

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"fmt"

	xenditprovider "github.com/livingdolls/payment-service/internal/provider/xendit"
)

type Service struct {
	repository         Repository
	xenditWebhookToken string
}

type ReceiveInput struct {
	Token   string
	Payload []byte
}

type ReceiveResult struct {
	Duplicate bool
	EventID   string
}

func NewService(repository Repository, xenditWebhookToken string) *Service {
	return &Service{
		repository:         repository,
		xenditWebhookToken: xenditWebhookToken,
	}
}

func (s *Service) ReceiveXenditPayment(ctx context.Context, input ReceiveInput) (*ReceiveResult, error) {
	if !s.verifyXenditToken(input.Token) {
		return nil, ErrInvalidToken
	}

	var payload xenditprovider.PaymentWebhook

	if err := json.Unmarshal(input.Payload, &payload); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPayload, err)
	}

	if payload.Event == "" || payload.Data.PaymentID == "" {
		return nil, ErrInvalidPayload
	}

	if !isSupportedPaymentEvent(payload.Event) {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedEvent, payload.Event)
	}

	eventKey := xenditprovider.PaymentWebhookEventKey(payload)

	providerPaymentID := payload.Data.PaymentID

	var providerPaymentRequestID *string

	if payload.Data.PaymentRequestID != "" {
		providerPaymentRequestID = &payload.Data.PaymentRequestID
	}

	var referenceID *string

	if payload.Data.ReferenceID != "" {
		referenceID = &payload.Data.ReferenceID
	}

	event := &Event{
		Provider:                 "XENDIT",
		EventKey:                 eventKey,
		EventType:                payload.Event,
		ProviderPaymentID:        &providerPaymentID,
		ProviderPaymentRequestID: providerPaymentRequestID,
		ReferenceID:              referenceID,
		Payload:                  input.Payload,
		Status:                   StatusReceived,
	}

	created, err := s.repository.Store(ctx, event)

	if err != nil {
		return nil, fmt.Errorf("store webhook: %w", err)
	}

	return &ReceiveResult{
		Duplicate: !created,
		EventID:   event.ID,
	}, nil
}

func (s *Service) verifyXenditToken(token string) bool {
	if token == "" {
		return false
	}

	return hmac.Equal(
		[]byte(token),
		[]byte(s.xenditWebhookToken),
	)
}

func isSupportedPaymentEvent(event string) bool {
	switch event {
	case "payment.capture", "payment.authorization", "payment.failure":
		return true
	default:
		return false
	}
}
