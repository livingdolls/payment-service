package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/livingdolls/payment-service/internal/database"
)

type ReconciliationRepository struct {
	db database.DBTX
}

type ReconciliationJob struct {
	AttemptID string

	ReconcileAttempts int
	ReconcileFailures int

	Generation int64
}

func NewReconciliationRepository(db database.DBTX) *ReconciliationRepository {
	return &ReconciliationRepository{
		db: db,
	}
}

func (r *ReconciliationRepository) ClaimNext(ctx context.Context, minAge time.Duration, cooldown time.Duration) (*ReconciliationJob, error) {
	const query = `
		WITH candidate AS (
			SELECT id
			FROM payment_attempts
			WHERE provider = 'XENDIT'
				AND reconcile_state = 'ACTIVE'
				AND status IN (
					'UNKNOWN',
					'REQUESTING_PROVIDER',
					'PENDING',
					'REQUIRES_ACTION',
					'AUTHORIZED'
				)
				AND updated_at <= NOW()
					- ($1::integer * INTERVAL '1 second')
					OR reconcile_force = TRUE
				AND (
					next_reconcile_at IS NULL
					OR next_reconcile_at <= NOW()
				)
			ORDER BY reconcile_force DESC, updated_at ASC, id ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)

		UPDATE payment_attempts a
		SET
			next_reconcile_at = NOW() + ($2::integer * INTERVAL '1 second'),
			reconcile_attempts = reconcile_attempts + 1,
			reconcile_generation = reconcile_generation + 1,
			reconcile_force = FALSE
		FROM candidate c
		WHERE a.id = c.id
		RETURNING
			a.id,
			a.reconcile_attempts,
			a.reconcile_failures,
			a.reconcile_generation
	`

	job := &ReconciliationJob{}

	err := r.db.QueryRow(
		ctx,
		query,
		int(minAge.Seconds()),
		int(cooldown.Seconds()),
	).Scan(
		&job.AttemptID,
		&job.ReconcileAttempts,
		&job.ReconcileFailures,
		&job.Generation,
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

	return job, nil
}

func (r *ReconciliationRepository) MarkFailure(ctx context.Context, job *ReconciliationJob, message string, delay time.Duration, maxFailures int) error {
	const query = `
		UPDATE payment_attempts
		SET
			reconcile_failures = reconcile_failures + 1,
			reconcile_state = 
				CASE
					WHEN reconcile_failures + 1 >= $3
						THEN 'NEEDS_REVIEW'
					ELSE 'ACTIVE'
				END,
			next_reconcile_at = 
				CASE
					WHEN reconcile_failures + 1 >= $3
						THEN NULL
					ELSE NOW() + ($4::integer * INTERVAL '1 second')
				END,
			last_reconcile_error = LEFT($5, 500)
		WHERE id = $1
			AND reconcile_generation = $2
			AND reconcile_state = 'ACTIVE'
			AND status IN (
   				'UNKNOWN',
			  	'REQUESTING_PROVIDER',
			  	'PENDING',
			  	'REQUIRES_ACTION',
			  	'AUTHORIZED'
			)
	`

	_, err := r.db.Exec(
		ctx,
		query,
		job.AttemptID,
		job.Generation,
		maxFailures,
		int(delay.Seconds()),
		message,
	)

	if err != nil {
		return fmt.Errorf(
			"mark reconciliation failure: %w",
			err,
		)
	}

	return nil
}

func (r *ReconciliationRepository) MarkSuccess(ctx context.Context, job *ReconciliationJob, cooldown time.Duration) error {
	const query = `
		UPDATE payment_attempts
		SET
			reconcile_failures = 0,
			last_reconcile_error = NULL,
			next_reconcile_at = 
				CASE
					WHEN status IN (
						'CAPTURED',
						'FAILED',
						'EXPIRED',
						'CANCELED'
					)
						THEN NULL
					ELSE NOW() + ($3::integer * INTERVAL '1 second')
				END
		WHERE id = $1
			AND reconcile_generation = $2
			AND reconcile_state = 'ACTIVE'
	`

	_, err := r.db.Exec(
		ctx,
		query,
		job.AttemptID,
		job.Generation,
		int(cooldown.Seconds()),
	)

	if err != nil {
		return fmt.Errorf(
			"mark reconciliation success: %w",
			err,
		)
	}

	return nil
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
