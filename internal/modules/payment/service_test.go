package payment

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepository struct {
	created *PaymentIntent
}

// Update implements [Repository].
func (r *fakeRepository) Update(ctx context.Context, payment *PaymentIntent, expectedVersion int64) error {
	panic("unimplemented")
}

var _ Repository = (*fakeRepository)(nil)

func (r *fakeRepository) Create(
	ctx context.Context,
	p *PaymentIntent,
) error {
	p.ID = "test-payment-id"
	p.Version = 1
	p.CreatedAt = time.Now()
	p.UpdatedAt = time.Now()

	r.created = p

	return nil
}

func (r *fakeRepository) GetByID(
	ctx context.Context,
	id string,
) (*PaymentIntent, error) {
	return nil, nil
}

func TestServiceCreate(t *testing.T) {
	repository := &fakeRepository{}

	service := NewService(repository)

	result, err := service.Create(
		context.Background(),
		CreateInput{
			OrderID:  "ORDER-001",
			Amount:   250000,
			Currency: "idr",
		},
	)

	if err != nil {
		t.Fatalf(
			"unexpected error: %v",
			err,
		)
	}

	if result.OrderID != "ORDER-001" {
		t.Errorf(
			"expected order id ORDER-001, got %s",
			result.OrderID,
		)
	}

	if result.Amount != 250000 {
		t.Errorf(
			"expected amount 250000, got %d",
			result.Amount,
		)
	}

	if result.Currency != "IDR" {
		t.Errorf(
			"expected currency IDR, got %s",
			result.Currency,
		)
	}

	if result.Status != StatusCreated {
		t.Errorf(
			"expected status CREATED, got %s",
			result.Status,
		)
	}

	if result.CaptureMethod != CaptureMethodAutomatic {
		t.Errorf(
			"expected AUTOMATIC, got %s",
			result.CaptureMethod,
		)
	}

	if result.ReferenceID == "" {
		t.Error(
			"expected reference id to be generated",
		)
	}
}

func TestServiceCreate_InvalidAmount(t *testing.T) {
	repository := &fakeRepository{}

	service := NewService(repository)

	_, err := service.Create(
		context.Background(),
		CreateInput{
			OrderID:  "ORDER-001",
			Amount:   0,
			Currency: "IDR",
		},
	)

	if !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf(
			"expected ErrInvalidAmount, got %v",
			err,
		)
	}

	if repository.created != nil {
		t.Fatal(
			"repository should not be called",
		)
	}
}

func TestServiceCreate_UnsupportedCurrency(
	t *testing.T,
) {
	repository := &fakeRepository{}

	service := NewService(repository)

	_, err := service.Create(
		context.Background(),
		CreateInput{
			OrderID:  "ORDER-001",
			Amount:   250000,
			Currency: "USD",
		},
	)

	if !errors.Is(
		err,
		ErrUnsupportedCurrency,
	) {
		t.Fatalf(
			"expected ErrUnsupportedCurrency, got %v",
			err,
		)
	}
}
