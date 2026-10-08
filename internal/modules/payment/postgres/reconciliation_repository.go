package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/livingdolls/payment-service/internal/database"
	"github.com/livingdolls/payment-service/internal/modules/payment"
)

type ReconciliationRepository struct {
	db database.DBTX
}

func NewReconciliationRepository(db database.DBTX) *ReconciliationRepository {
	return &ReconciliationRepository{
		db: db,
	}
}

func (r *ReconciliationRepository) ClaimNext(ctx context.Context, minAge time.Duration, cooldown time.Duration) (*payment.PaymentAttempt, error) {
	const query = `
		WITH candidate AS (
			SELECT id
			FROM payment_attempts
			WHERE provider = 'XENDIT'
				AND provider_payment_request_id IS NOT NULL
				AND status IN (
					'UNKNOWN',
					'REQUESTING_PROVIDER',
					'PENDING',
					'REQUIRES_ACTION',
					'AUTHORIZED'
				)
				AND updated_at <= NOW()
					- ($1::integer * INTERVAL '1 second')
				AND (
					next_reconcile_at IS NULL
					OR next_reconcile_at <= NOW()
				)
			ORDER BY updated_at ASC, id ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)

		UPDATE payment_attempts a
		SET
			next_reconcile_at = NOW() + ($2::integer * INTERVAL '1 second'),
			reconcile_attempts = reconcile_attempts + 1,
			last_reconcile_error = NULL
		FROM candidate c
		WHERE a.id = c.id
		RETURNING
			a.id,
			a.payment_intent_id,
			a.attempt_number,
			a.provider,
			a.provider_idempotency_key,
			a.provider_payment_request_id,
			a.provider_payment_id,
			a.status,
			a.error_code,
			a.error_message,
			a.provider_request,
			a.provider_response,
			a.version,
			a.created_at,
			a.updated_at
	`

	attempt := &payment.PaymentAttempt{}

	err := r.db.QueryRow(
		ctx,
		query,
		int(minAge.Seconds()),
		int(cooldown.Seconds()),
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

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf(
			"claim payment reconciliation: %w",
			err,
		)
	}

	return attempt, nil
}

func (r *ReconciliationRepository) RecordError(ctx context.Context, attemptID string, message string) error {
	const query = `
		UPDATE payment_attempts
		SET last_reconcile_error = $2
		WHERE id = $1
	`

	_, err := r.db.Exec(
		ctx,
		query,
		attemptID,
		message,
	)

	if err != nil {
		return fmt.Errorf(
			"record reconciliation error: %w",
			err,
		)
	}

	return nil
}
