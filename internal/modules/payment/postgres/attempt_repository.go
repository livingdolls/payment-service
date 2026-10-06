package postgres

import (
	"context"
	"fmt"

	"github.com/livingdolls/payment-service/internal/database"
	"github.com/livingdolls/payment-service/internal/modules/payment"
)

type AttemptRepository struct {
	db database.DBTX
}

var _ payment.AttemptRepository = (*AttemptRepository)(nil)

func NewAttemptRepository(db database.DBTX) *AttemptRepository {
	return &AttemptRepository{
		db: db,
	}
}

// CreateAttempt implements [payment.AttemptRepository].
func (a *AttemptRepository) CreateAttempt(ctx context.Context, attempt *payment.PaymentAttempt) error {
	const query = `
		INSERT INTO payment_attempts (
			payment_intent_id,
			attempt_number,
			provider,
			provider_idempotency_key,
			status
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING
			id,
			created_at,
			updated_at
	`

	err := a.db.QueryRow(
		ctx,
		query,
		attempt.PaymentIntentID,
		attempt.AttemptNumber,
		attempt.Provider,
		attempt.ProviderIdempotencyKey,
		attempt.Status,
	).Scan(
		&attempt.ID,
		&attempt.CreatedAt,
		&attempt.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("create payment attempt: %w", err)
	}

	return nil
}

// GetAttemptByID implements [payment.AttemptRepository].
func (a *AttemptRepository) GetAttemptByID(ctx context.Context, id string) (*payment.PaymentAttempt, error) {
	const query = `
		SELECT
			id,
			payment_intent_id,
			attempt_number,
			provider,
			provider_idempotency_key,
			provider_payment_request_id,
			provider_payment_id,
			status,
			error_code,
			error_message,
			provider_response,
			created_at,
			updated_at
		FROM payment_attempts
		WHERE id = $1
	`

	attempt := &payment.PaymentAttempt{}

	err := a.db.QueryRow(
		ctx,
		query,
		id,
	).Scan(
		&attempt.ID,
		&attempt.PaymentIntentID,
		&attempt.AttemptNumber,
		&attempt.Provider,
		&attempt.ProviderIdempotencyKey,
		&attempt.ProviderPaymentRequestID,
		&attempt.ProviderPaymentID,
		&attempt.Status,
		&attempt.ErrorCode,
		&attempt.ErrorMessage,
		&attempt.ProviderResponse,
		&attempt.CreatedAt,
		&attempt.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("get payment attempt by id: %w", err)
	}

	return attempt, nil
}

// GetLatestAttempt implements [payment.AttemptRepository].
func (a *AttemptRepository) GetLatestAttempt(ctx context.Context, paymentIntentId string) (*payment.PaymentAttempt, error) {
	const query = `
		SELECT
			id,
			payment_intent_id,
			attempt_number,
			provider,
			provider_idempotency_key,
			provider_payment_request_id,
			provider_payment_id,
			status,
			error_code,
			error_message,
			provider_response,
			created_at,
			updated_at
		FROM payment_attempts
		WHERE payment_intent_id = $1
		ORDER BY attempt_number DESC
		LIMIT 1
	`

	attempt := &payment.PaymentAttempt{}

	err := a.db.QueryRow(
		ctx,
		query,
		paymentIntentId,
	).Scan(
		&attempt.ID,
		&attempt.PaymentIntentID,
		&attempt.AttemptNumber,
		&attempt.Provider,
		&attempt.ProviderIdempotencyKey,
		&attempt.ProviderPaymentRequestID,
		&attempt.ProviderPaymentID,
		&attempt.Status,
		&attempt.ErrorCode,
		&attempt.ErrorMessage,
		&attempt.ProviderResponse,
		&attempt.CreatedAt,
		&attempt.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"get latest payment attempt: %w", err,
		)
	}

	return attempt, nil
}
