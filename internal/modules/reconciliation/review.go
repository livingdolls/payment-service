package reconciliation

import (
	"context"
	"errors"
	"time"

	"github.com/livingdolls/payment-service/internal/modules/payment"
)

var (
	ErrReviewNotFound   = errors.New("payment attempt not found")
	ErrNotNeedsReview   = errors.New("payment attempt is not eligible for reconciliation")
	ErrNotEligible      = errors.New("payment attempt is not eligible for reconciliation")
	ErrInvalidReason    = errors.New("invalid review reason")
	ErrInvalidAttemptID = errors.New("invalid payment attempt id")
	ErrInvalidActor     = errors.New("invalid admin actor")
)

type ReviewItem struct {
	AttemptID string `json:"attempt_id"`

	OrderID     string `json:"order_id"`
	ReferenceID string `json:"reference_id"`

	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`

	PaymentStatus payment.Status        `json:"payment_status"`
	AttemptStatus payment.AttemptStatus `json:"attempt_status"`

	ProviderPaymentRequestID *string `json:"provider_payment_request_id"`

	ReconcileState    string `json:"reconcile_state"`
	ReconcileAttempts int    `json:"reconcile_attempts"`
	ReconcileFailures int    `json:"reconcile_failures"`

	LastReconcileError *string    `json:"last_reconcile_error"`
	NextReconcileAt    *time.Time `json:"next_reconcile_at"`
}

type ReviewAction struct {
	ID        string `json:"id"`
	AttemptID string `json:"attempt_id"`

	Actor  string `json:"actor"`
	Action string `json:"action"`
	Reason string `json:"reason"`

	PreviousState string `json:"previous_state"`
	NewState      string `json:"new_state"`

	CreatedAt time.Time `json:"created_at"`
}

type Repository interface {
	List(ctx context.Context, limit int) ([]ReviewItem, error)
	Requeue(ctx context.Context, attemptID string, actor string, reason string) (*ReviewAction, error)
}
