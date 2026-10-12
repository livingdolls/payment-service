package order

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/livingdolls/payment-service/internal/modules/outbox"
)

type Consumer struct {
	repository Repository
}

func NewConsumer(repository Repository) *Consumer {
	return &Consumer{
		repository: repository,
	}
}

func (c *Consumer) Consume(ctx context.Context, event outbox.PaymentCapturedPayload) (bool, error) {
	eventID, err := uuid.Parse(event.EventID)
	if err != nil {
		return false, fmt.Errorf("%w: invalid event id", ErrInvalidEvent)
	}

	paymentID, err := uuid.Parse(event.PaymentIntentID)

	if err != nil {
		return false, fmt.Errorf("%w: invalid payment intent id", ErrInvalidEvent)
	}

	if event.EventType != outbox.EventPaymentCaptured {
		return false, fmt.Errorf("%w: unsuported event type", ErrInvalidEvent)
	}

	if strings.TrimSpace(event.OrderID) == "" || strings.TrimSpace(event.ReferenceID) == "" {
		return false, fmt.Errorf("%w: missing order or reference id", ErrInvalidEvent)
	}

	if event.Amount <= 0 || event.CapturedAmount != event.Amount {
		return false, fmt.Errorf("%w: invalid captured amount", ErrInvalidEvent)
	}

	if event.Currency != "IDR" {
		return false, fmt.Errorf("%w: unsupported currency", ErrInvalidEvent)
	}

	// Canonicalize UUID sebelum membuat hash.
	event.EventID = eventID.String()
	event.PaymentIntentID = paymentID.String()

	cannonicalPayload, err := json.Marshal(event)
	if err != nil {
		return false, fmt.Errorf("marshal payment event: %w", err)
	}

	digest := sha256.Sum256(cannonicalPayload)
	payloadHash := hex.EncodeToString(digest[:])

	return c.repository.ConsumePaymentCaptured(ctx, event, payloadHash)
}
