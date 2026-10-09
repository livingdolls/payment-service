package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/livingdolls/payment-service/internal/database"
	"github.com/livingdolls/payment-service/internal/modules/outbox"
)

type Repository struct {
	db database.DBTX
}

var _ outbox.Repository = (*Repository)(nil)

func NewRepository(db database.DBTX) *Repository {
	return &Repository{
		db: db,
	}
}

// ClaimNext implements [outbox.Repository].
func (r *Repository) ClaimNext(ctx context.Context, lease time.Duration) (*outbox.Event, error) {
	const query = `
		WITH candidate AS (
			SELECT id
			FROM outbox_events
			WHERE (
				status = 'PENDING'
				AND available_at <= NOW()
			)
			OR (
				status = 'PROCESSING'
				AND claimed_at <
					NOW() - ($1::integer * INTERVAL '1 second')
			)

			ORDER BY available_at ASC, created_at ASC, id ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)

		UPDATE outbox_events AS o
		SET
			status = 'PROCESSING',
			attempt_count = o.attempt_count + 1,
			claim_token = gen_random_uuid(),
			claimed_at = NOW(),
			updated_at = NOW(),
			last_error = NULL
		FROM candidate c
		WHERE o.id = c.id
		RETURNING
			o.id,
			o.event_key,
			o.aggregate_type,
			o.aggregate_id,
			o.event_type,
			o.payload,
			o.status,
			o.attempt_count,
			o.available_at,
			o.claim_token,
			o.claimed_at,
			o.published_at,
			o.last_error,
			o.created_at,
			o.updated_at
	`

	event := &outbox.Event{}

	err := r.db.QueryRow(
		ctx,
		query,
		int(lease.Seconds()),
	).Scan(
		&event.ID,
		&event.EventKey,
		&event.AggregateType,
		&event.AggregateID,
		&event.EventType,
		&event.Payload,
		&event.Status,
		&event.AttemptCount,
		&event.AvailableAt,
		&event.ClaimToken,
		&event.ClaimedAt,
		&event.PublishedAt,
		&event.LastError,
		&event.CreatedAt,
		&event.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("claim outbox event: %w", err)
	}

	return event, nil
}

// MarkDead implements [outbox.Repository].
func (r *Repository) MarkDead(ctx context.Context, id string, claimToken string, message string) error {
	const query = `
		UPDATE outbox_events
		SET
			status = 'DEAD',
			claim_token = NULL,
			claimed_at = NULL,
			last_error = LEFT($3, 1000),
			updated_at = NOW
		WHERE id = $1
			AND claim_token = $::uuid
			AND status = 'PROCESSING'
	`

	result, err := r.db.Exec(
		ctx,
		query,
		id,
		claimToken,
		message,
	)

	if err != nil {
		return fmt.Errorf("mark outbox dead: %w", err)
	}

	if result.RowsAffected() != 1 {
		return outbox.ErrClaimLost
	}

	return nil
}

// MarkForRetry implements [outbox.Repository].
func (r *Repository) MarkForRetry(ctx context.Context, id string, claimToken string, maxAttempts int, delay time.Duration, message string) (outbox.Status, error) {
	const query = `
		UPDATE outbox_events
		SET
			status = 
				CASE
					WHEN attempt_count >= $3
						THEN 'DEAD'
					ELSE 'PENDING'
				END,

			available_at = 
				CASE
					WHEN attempt_count >= $3
						THEN available_at
					ELSE NOW() + ($4::integer * INTERVAL '1 Second')
				END,

				claim_token = NULL,
				claimed_at = NULL,

				last_error = LEFT($5, 1000),
				updated_at = NOW()
		
		WHERE id = $1
			AND claim_token = $2::uuid
			AND status = 'PROCESSING'

		RETURNING status
	`

	var status outbox.Status

	err := r.db.QueryRow(
		ctx,
		query,
		id,
		claimToken,
		maxAttempts,
		int(delay.Seconds()),
		message,
	).Scan(&status)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", outbox.ErrClaimLost
	}

	if err != nil {
		return "", fmt.Errorf("mark outbox for retry: %w", err)
	}

	return status, nil
}

// MarkPublished implements [outbox.Repository].
func (r *Repository) MarkPublished(ctx context.Context, id string, claimToken string) error {
	const query = `
		UPDATE outbox_events
		SET
			status = 'PUBLISHED'
			published_at = NOW(),
			claim_token = NULL,
			claimed_at = NULL,
			last_error = NULL,
			updated_at = NOW()
		WHERE id = $1
			AND claim_token = $2::uuid
			AND status = 'PROCESSING'
	`

	result, err := r.db.Exec(
		ctx,
		query,
		id,
		claimToken,
	)

	if err != nil {
		return fmt.Errorf("mark outbox published: %w", err)
	}

	if result.RowsAffected() != 1 {
		return outbox.ErrClaimLost
	}

	return nil
}
