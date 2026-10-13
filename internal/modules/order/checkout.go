package order

import (
	"context"
	"crypto/sha256"
	"fmt"
)

type CheckoutOrder struct {
	ID string

	Amount   int64
	Currency string

	Status string

	PaymentIntentID *string
}

type CheckoutPayment struct {
	PaymentIntentID string
	AttemptID       string

	ReferenceID string
	OrderID     string

	Amount   int64
	Currency string

	Status string
}

type CheckoutResult struct {
	OrderID string `json:"order_id"`

	PaymentIntentID string `json:"payment_intent_id"`
	AttemptID       string `json:"attempt_id"`
	ReferenceID     string `json:"reference_id"`

	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`

	OrderStatus   string `json:"order_status"`
	PaymentStatus string `json:"payment_status"`
}

type CheckoutRepository interface {
	GetForCheckout(ctx context.Context, orderID string) (*CheckoutOrder, error)
	BindPaymentIntent(ctx context.Context, order CheckoutOrder, paymentIntentID string) error
}

type PaymentCreator interface {
	CreatePayment(ctx context.Context, checkoutOrder CheckoutOrder, idempotencyKey string) (*CheckoutPayment, error)
}

type CheckoutService struct {
	orders   CheckoutRepository
	payments PaymentCreator
}

func NewCheckoutService(
	orders CheckoutRepository,
	payments PaymentCreator,
) *CheckoutService {
	return &CheckoutService{
		orders:   orders,
		payments: payments,
	}
}

func (s *CheckoutService) Start(ctx context.Context, orderID string) (*CheckoutResult, error) {
	order, err := s.orders.GetForCheckout(
		ctx,
		orderID,
	)

	if err != nil {
		return nil, err
	}

	if order.Status != "PENDING_PAYMENT" {
		return nil, ErrOrderNotPayable
	}

	if order.Amount <= 0 || order.Currency != "IDR" {
		return nil, ErrCheckoutOrderChanged
	}

	hash := sha256.Sum256(
		[]byte("checkout:create-payment:v1:" + order.ID),
	)

	idempotencyKey := fmt.Sprintf("checkout-v1-%x", hash)

	payment, err := s.payments.CreatePayment(
		ctx,
		*order,
		idempotencyKey,
	)

	if err != nil {
		return nil, err
	}

	if payment.OrderID != order.ID || payment.Amount != order.Amount || payment.Currency != order.Currency {
		return nil, ErrCheckoutPaymentMismatch
	}

	if order.PaymentIntentID != nil && *order.PaymentIntentID != payment.PaymentIntentID {
		return nil, ErrCheckoutAlreadyBound
	}

	if err := s.orders.BindPaymentIntent(
		ctx,
		*order,
		payment.PaymentIntentID,
	); err != nil {
		return nil, err
	}

	return &CheckoutResult{
		OrderID: order.ID,

		PaymentIntentID: payment.PaymentIntentID,
		AttemptID:       payment.AttemptID,
		ReferenceID:     payment.ReferenceID,

		Amount:        order.Amount,
		Currency:      order.Currency,
		OrderStatus:   order.Status,
		PaymentStatus: payment.Status,
	}, nil
}
