package payment

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type CreateInput struct {
	OrderID  string
	Amount   int64
	Currency string
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{
		repository: repository,
	}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*PaymentIntent, error) {
	orderID := strings.TrimSpace(input.OrderID)

	if orderID == "" {
		return nil, ErrOrderIDRequired
	}

	if len(orderID) > 100 {
		return nil, ErrOrderIDTooLong
	}

	if input.Amount <= 0 {
		return nil, ErrInvalidAmount
	}

	currency := strings.ToUpper(strings.TrimSpace(input.Currency))

	if len(currency) != 3 {
		return nil, ErrInvalidCurrency
	}

	if currency != "IDR" {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedCurrency, currency)
	}

	paymentIntent := &PaymentIntent{
		ReferenceID: generateReferenceID(),
		OrderID:     orderID,

		Amount:   input.Amount,
		Currency: currency,

		Status:        StatusCreated,
		CaptureMethod: CaptureMethodAutomatic,
	}

	if err := s.repository.Create(ctx, paymentIntent); err != nil {
		return nil, fmt.Errorf("create payment intent: %w", err)
	}

	return paymentIntent, nil
}

func generateReferenceID() string {
	return "PAY-" + strings.ToUpper(uuid.NewString())
}
