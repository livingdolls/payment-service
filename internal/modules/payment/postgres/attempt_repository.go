package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
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
			version,
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
		&attempt.Version,
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
			provider_request,
			provider_response,
			version,
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
		&attempt.ProviderRequest,
		&attempt.ProviderResponse,
		&attempt.Version,
		&attempt.CreatedAt,
		&attempt.UpdatedAt,
	)

	if err == nil {
		return attempt, nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, payment.ErrAttemptNotFound
	}

	return nil, fmt.Errorf("get payment attempt by id: %w", err)
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
			provider_request,
			provider_response,
			version,
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
		&attempt.ProviderRequest,
		&attempt.ProviderResponse,
		&attempt.Version,
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

func (a *AttemptRepository) UpdateAttempt(ctx context.Context, attempt *payment.PaymentAttempt, expectedVersion int64) error {
	const query = `
		UPDATE payment_attempts
		SET
			provider_payment_request_id = $1,
			provider_payment_id = $2,
			status = $3,
			error_code = $4,
			error_message = $5,
			provider_request = $9,
			provider_response = $6::jsonb,

			version = version + 1,
			updated_at = NOW()
		WHERE id = $7
			AND version = $8
		RETURNING
			version,
			updated_at
	`

	err := a.db.QueryRow(
		ctx,
		query,

		attempt.ProviderPaymentRequestID,
		attempt.ProviderPaymentID,
		attempt.Status,
		attempt.ErrorCode,
		attempt.ErrorMessage,
		attempt.ProviderResponse,
		attempt.ID,
		expectedVersion,
		attempt.ProviderRequest,
	).Scan(
		&attempt.Version,
		&attempt.UpdatedAt,
	)

	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return payment.ErrAttemptConcurrentUpdate
	}

	return fmt.Errorf("update payment attempt: %w", err)
}
