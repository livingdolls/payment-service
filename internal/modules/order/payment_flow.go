package order

import (
	"context"
	"errors"
)

var (
	ErrCheckoutRequired           = errors.New("order has no payment intent")
	ErrUnsupportedChannel         = errors.New("unsupported payment channel")
	ErrPaymentProcessingRejected  = errors.New("payment processing rejected")
	ErrPaymentProcessingUncertain = errors.New("payment processing outcome uncertain")
)

type PaymentAction struct {
	Type       string `json:"type"`
	Descriptor string `json:"descriptor"`
	Value      string `json:"value"`
}

type PaymentSnapshot struct {
	PaymentIntentID string `json:"payment_intent_id"`
	AttemptID       string `json:"attempt_id"`

	ReferenceID string `json:"reference_id"`
	OrderID     string `json:"order_id"`

	Amount         int64  `json:"amount"`
	Currency       string `json:"currency"`
	CapturedAmount int64  `json:"captured_amount"`

	PaymentStatus string `json:"payment_status"`
	AttemptStatus string `json:"attempt_status"`

	Actions []PaymentAction `json:"actions"`
}

type OrderPaymentStatus struct {
	OrderID     string `json:"order_id"`
	OrderStatus string `json:"order_status"`

	Payment *PaymentSnapshot `json:"payment"`
}

type PaymentFlowGateway interface {
	GetStatus(ctx context.Context, paymentID string) (*PaymentSnapshot, error)
	Process(ctx context.Context, attemptID string, channelCode string) error
}

type PaymentFlowService struct {
	orders   CheckoutRepository
	payments PaymentFlowGateway
}

func NewPaymentFlowService(orders CheckoutRepository, payments PaymentFlowGateway) *PaymentFlowService {
	return &PaymentFlowService{
		orders:   orders,
		payments: payments,
	}
}

func (s *PaymentFlowService) Status(ctx context.Context, orderID string, customerID string) (*OrderPaymentStatus, error) {
	checkoutOrder, err := s.orders.GetForCheckout(ctx, orderID)

	if err != nil {
		return nil, err
	}

	if err := RequireOrderOwner(checkoutOrder, customerID); err != nil {
		return nil, err
	}

	if checkoutOrder.PaymentIntentID == nil {
		return nil, ErrCheckoutRequired
	}

	payment, err := s.payments.GetStatus(ctx, *checkoutOrder.PaymentIntentID)

	if err != nil {
		return nil, err
	}

	if payment.PaymentIntentID != *checkoutOrder.PaymentIntentID || payment.OrderID != checkoutOrder.ID || payment.Amount != checkoutOrder.Amount || payment.Currency != checkoutOrder.Currency {
		return nil, ErrCheckoutPaymentMismatch
	}

	return &OrderPaymentStatus{
		OrderID:     checkoutOrder.ID,
		OrderStatus: checkoutOrder.Status,
		Payment:     payment,
	}, nil
}

func (s *PaymentFlowService) Pay(ctx context.Context, orderID string, customerID string, channelCode string) (*OrderPaymentStatus, error) {
	if channelCode != "QRIS" {
		return nil, ErrUnsupportedChannel
	}

	current, err := s.Status(ctx, orderID, customerID)
	if err != nil {
		return nil, err
	}

	if current.OrderStatus != "PENDING_PAYMENT" {
		return nil, ErrOrderNotPayable
	}

	attempt := current.Payment

	if attempt.AttemptStatus != "CREATED" {
		return current, nil
	}

	processErr := s.payments.Process(ctx, attempt.AttemptID, channelCode)

	latest, err := s.Status(ctx, orderID, customerID)
	if err != nil {
		return nil, err
	}

	if processErr != nil && latest.Payment.AttemptStatus == "CREATED" {
		return nil, processErr
	}

	return latest, nil
}
