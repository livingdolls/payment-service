package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/livingdolls/payment-service/internal/database"
	"github.com/livingdolls/payment-service/internal/modules/webhook"
)

type Repository struct {
	db database.DBTX
}

var _ webhook.Repository = (*Repository)(nil)

func NewRepository(db database.DBTX) *Repository {
	return &Repository{
		db: db,
	}
}

// Store implements [webhook.Repository].
func (r *Repository) Store(ctx context.Context, event *webhook.Event) (bool, error) {
	const query = `
		INSERT INTO webhook_events (
			provider,
			event_key,
			event_type,

			provider_payment_id,
			provider_payment_request_id,
			reference_id,

			payload,
			status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8)

		ON CONFLICT (
			provider,
			event_key
		)
		DO NOTHING
		RETURNING
			id,
			received_at
	`

	err := r.db.QueryRow(
		ctx,
		query,
		event.Provider,
		event.EventKey,
		event.EventType,

		event.ProviderPaymentID,
		event.ProviderPaymentRequestID,
		event.ReferenceID,

		event.Payload,
		webhook.StatusReceived,
	).Scan(
		&event.ID,
		&event.ReceivedAt,
	)

	if err == nil {
		event.Status = webhook.StatusReceived

		return true, nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		// ON CONFLICT DO NOTHIN
		return false, nil
	}

	return false, fmt.Errorf(
		"store webhook event: %w", err,
	)
}

// ClaimNext implements [webhook.Repository].
func (r *Repository) ClaimNext(ctx context.Context, staleAfter time.Duration) (*webhook.Event, error) {
	const query = `
		WITH candidate AS (
			SELECT id
			FROM webhook_events
			WHERE
				status = $1
				OR (
					status = $2
					AND processing_started_at
						< NOW() - ($3 * INTERVAL '1 second')
				)
			ORDER BY received_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)

		UPDATE webhook_events AS w
		SET
			status = $2,
			processing_attempts = processing_attempts + 1,
			processing_started_at = NOW(),
			error_message = NULL
		FROM candidate
		WHERE w.id = candidate.id
		RETURNING
			w.id,
			w.provider,
			w.event_key,
			w.event_type,
			w.provider_payment_id,
			w.provider_payment_request_id,
			w.reference_id,
			w.payload,
			w.status,
			w.processing_attempts,
			w.error_message,
			w.received_at,
			w.processing_started_at,
			w.processed_at
	`

	event := &webhook.Event{}

	err := r.db.QueryRow(
		ctx,
		query,
		webhook.StatusReceived,
		webhook.StatusProcessing,
		int64(staleAfter.Seconds()),
	).Scan(
		&event.ID,
		&event.Provider,
		&event.EventKey,
		&event.EventType,
		&event.ProviderPaymentID,
		&event.ProviderPaymentRequestID,
		&event.ReferenceID,
		&event.Payload,
		&event.Status,
		&event.ProcessingAttempts,
		&event.ErrorMessage,
		&event.ReceivedAt,
		&event.ProcessingStartedAt,
		&event.ProcessedAt,
	)

	if err == nil {
		return event, nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, webhook.ErrNoEventAvailable
	}

	return nil, fmt.Errorf("claim webhook event: %w", err)
}

// MarkForRetry implements [webhook.Repository].
func (r *Repository) MarkForRetry(ctx context.Context, id string, message string, maxAttempts int) error {
	const query = `
		UPDATE webhook_events
		SET
			status = CASE
				WHEN processing_attempts >= $1
					THEN $2
				ELSE $3
			END,

			error_message = $4,
			processing_started_at = NULL

		WHERE id = $5
			AND status = $6
	`

	result, err := r.db.Exec(
		ctx,
		query,
		maxAttempts,
		webhook.StatusFailed,
		webhook.StatusReceived,
		message,
		id,
		webhook.StatusProcessing,
	)

	if err != nil {
		return fmt.Errorf("mark webhook for retry: %w", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("mark webhook for retry: event is not processing")
	}

	return nil
}

// MarkProcessed implements [webhook.Repository].
func (r *Repository) MarkProcessed(ctx context.Context, id string) error {
	const query = `
		UPDATE webhook_events
		SET
			status = $1,
			processed_at = NOW(),
			processing_started_at = NULL,
			error_message = NULL
		WHERE id = $2
			AND status = $3
	`

	result, err := r.db.Exec(
		ctx,
		query,
		webhook.StatusProcessed,
		id,
		webhook.StatusProcessing,
	)

	if err != nil {
		return fmt.Errorf("mark webhook processed: %w", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("mark webhook: event is not processing")
	}

	return nil
}
