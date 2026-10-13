package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/livingdolls/payment-service/internal/modules/order"
	"github.com/livingdolls/payment-service/internal/modules/outbox"
)

type Repository struct {
	db *pgxpool.Pool
}

var _ order.Repository = (*Repository)(nil)

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db: db,
	}
}

// ConsumePaymentCaptured implements [order.Repository].
func (r *Repository) ConsumePaymentCaptured(ctx context.Context, event outbox.PaymentCapturedPayload, payloadHash string) (duplicate bool, err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin order transaction: %w", err)
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	const selectOrder = `
		SELECT
			amount,
			currency,
			status,
			payment_intent_id
		FROM orders
		WHERE id = $1
		FOR UPDATE
	`

	var expectedAmount int64
	var expectedCurrency string
	var currentStatus string
	var expectedPaymentIntentID *string

	err = tx.QueryRow(
		ctx,
		selectOrder,
		event.OrderID,
	).Scan(
		&expectedAmount,
		&expectedCurrency,
		&currentStatus,
		&expectedPaymentIntentID,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return false, order.ErrOrderNotFound
	}

	if err != nil {
		return false, fmt.Errorf("lock order: %w", err)
	}

	if expectedPaymentIntentID == nil {
		return false, order.ErrPaymentNotBound
	}

	if *expectedPaymentIntentID != event.PaymentIntentID {
		return false, order.ErrPaymentMismatch
	}

	if expectedAmount != event.Amount || expectedCurrency != event.Currency {
		return false, order.ErrPaymentMismatch
	}

	switch currentStatus {
	case "PENDING_PAYMENT", "PAID":
	default:
		return false, order.ErrOrderNotPayable
	}

	const insertEvent = `
		INSERT INTO consumed_payment_events (
			event_id,
			event_type,
			order_id,
			payment_intent_id,
			payload_hash
		)
		VALUES (
			$1::uuid,
			$2,
			$3,
			$4::uuid,
			$5
		)
		ON CONFLICT (event_id)
		DO NOTHING
		RETURNING event_id
	`

	var insertedEventID string

	err = tx.QueryRow(
		ctx,
		insertEvent,
		event.EventID,
		event.EventType,
		event.OrderID,
		event.PaymentIntentID,
		payloadHash,
	).Scan(
		&insertedEventID,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		// event ID sudah pernah di konsumsi
		// pastikan payload benar-benar sama

		var existingHash string

		err = tx.QueryRow(
			ctx,
			`
				SELECT payload_hash
				FROM consumed_payment_events
				WHERE event_id = $1::uuid	
			`,
			event.EventID,
		).Scan(&existingHash)

		if err != nil {
			return false, fmt.Errorf("read consumed event: %w", err)
		}

		if existingHash != payloadHash {
			return false, order.ErrEventPayloadConflict
		}

		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit duplicate event: %w", err)
		}

		return true, nil
	}

	if err != nil {
		return false, fmt.Errorf("insert consumed event: %w", err)
	}

	if currentStatus == "PENDING_PAYMENT" {
		const updateOrder = `
			UPDATE orders
			SET
				status = 'PAID',
				paid_at = NOW(),
				updated_at = NOW()
			WHERE id = $1
				AND status = 'PENDING_PAYMENT'
		`

		result, err := tx.Exec(
			ctx,
			updateOrder,
			event.OrderID,
		)

		if err != nil {
			return false, fmt.Errorf("mark order paid: %w", err)
		}

		if result.RowsAffected() != 1 {
			return false, order.ErrOrderNotPayable
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit consumed payment event: %w", err)
	}

	return false, nil
}
