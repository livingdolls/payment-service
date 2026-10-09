package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/livingdolls/payment-service/internal/database"
	"github.com/livingdolls/payment-service/internal/modules/payment"
	"github.com/livingdolls/payment-service/internal/modules/reconciliation"
)

type ReviewRepository struct {
	db                 database.DBTX
	transactionManager database.TransactionManager
}

var _ reconciliation.Repository = (*ReviewRepository)(nil)

func NewReviewRepository(db database.DBTX, transactionManager database.TransactionManager) *ReviewRepository {
	return &ReviewRepository{
		db:                 db,
		transactionManager: transactionManager,
	}
}

// List implements [reconciliation.Repository].
func (r *ReviewRepository) List(ctx context.Context, limit int) ([]reconciliation.ReviewItem, error) {
	const query = `
		SELECT
			a.id,
			p.order_id,
			p.reference_id,
			p.amount,
			p.currency,
			p.status,
			a.status,
			a.provider_payment_request_id,
			a.reconcile_state,
			a.reconcile_attempts,
			a.reconcile_failures,
			a.last_reconcile_error,
			a.next_reconcile_at
		FROM payment_attempts a
		JOIN payment_intents p
			ON p.id = a.payment_intent_id
		WHERE a.reconcile_state = 'NEEDS_REVIEW'
			AND a.status IN (
 			  'UNKNOWN',
			  'REQUESTING_PROVIDER',
			  'PENDING',
			  'REQUIRES_ACTION',
			  'AUTHORIZED'
			)
		ORDER BY a.updated_at ASC, a.id ASC
		LIMIT $1
	`

	rows, err := r.db.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf(
			"list reconciliation reviews: %w",
			err,
		)
	}

	if err != nil {
		return nil, fmt.Errorf("list reconciliation reviews: %w", err)
	}

	defer rows.Close()

	items := make([]reconciliation.ReviewItem, 0)

	for rows.Next() {
		var item reconciliation.ReviewItem

		if err := rows.Scan(
			&item.AttemptID,
			&item.OrderID,
			&item.ReferenceID,
			&item.Amount,
			&item.Currency,
			&item.PaymentStatus,
			&item.AttemptStatus,
			&item.ProviderPaymentRequestID,
			&item.ReconcileState,
			&item.ReconcileAttempts,
			&item.ReconcileFailures,
			&item.LastReconcileError,
			&item.NextReconcileAt,
		); err != nil {
			return nil, fmt.Errorf("scan reconciliation review: %w", err)
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read reconciliation reviews: %w", err)
	}

	return items, nil
}

// Requeue implements [reconciliation.Repository].
func (r *ReviewRepository) Requeue(ctx context.Context, attemptID string, actor string, reason string) (*reconciliation.ReviewAction, error) {
	action := &reconciliation.ReviewAction{
		AttemptID: attemptID,
		Actor:     actor,
		Action:    "REQUEUE",
		Reason:    reason,
	}

	err := r.transactionManager.WithinTransaction(
		ctx,
		func(db database.DBTX) error {
			const selectQuery = `
				SELECT
					status,
					reconcile_state,
					reconcile_failures,
				FROM payment_attempts
				WHERE id = $1
				FOR UPDATE			
			`

			var status payment.AttemptStatus
			var reconcileState string
			var failures int

			err := db.QueryRow(
				ctx,
				selectQuery,
				attemptID,
			).Scan(
				&status,
				&reconcileState,
				&failures,
			)

			if errors.Is(err, pgx.ErrNoRows) {
				return reconciliation.ErrReviewNotFound
			}

			if err != nil {
				return fmt.Errorf("lock payment attempt: %w", err)
			}

			switch status {
			case payment.AttemptStatusUnknown,
				payment.AttemptStatusRequestingProvider,
				payment.AttemptStatusPending,
				payment.AttemptStatusRequiresAction,
				payment.AttemptStatusAuthorized:
				// eligible for reconciliation

			default:
				return reconciliation.ErrNotEligible
			}

			if reconcileState != "NEEDS_REVIEW" {
				return reconciliation.ErrNotNeedsReview
			}

			action.PreviousState = reconcileState
			action.NewState = "ACTIVE"

			const updateQuery = `
				UPDATE payment_attempts
				SET
					reconcile_state = 'ACTIVE',
					reconcile_failures = 0,
					reconcile_force = TRUE,
					next_reconcile_at = NOW(),
					last_reconcile_error = NULL,
					reconcile_generation = reconcile_generation + 1
				WHERE id = $1
			`

			if _, err := db.Exec(
				ctx,
				updateQuery,
				attemptID,
			); err != nil {
				return fmt.Errorf(
					"requeue reconciliation: %w",
					err,
				)
			}

			const auditQuery = `
				INSERT INTO payment_reconciliation_actions (
					payment_attempt_id,
					actor,
					action,
					reason,
					previous_state,
					new_state,
					previous_failures
				)
				VALUES ($1, $2, $3, $5, $5, $6, $7)
				RETURNING
					id,
					created_at
			`

			if err := db.QueryRow(
				ctx,
				auditQuery,
				attemptID,
				actor,
				action.Action,
				reason,
				action.PreviousState,
				action.NewState,
				failures,
			).Scan(
				&action.ID,
				&action.CreatedAt,
			); err != nil {
				return fmt.Errorf("insert reconciliation audit: %w", err)
			}

			return nil
		},
	)

	if err != nil {
		return nil, err
	}

	return action, nil
}
