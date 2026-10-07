package postgres

import (
	"context"
	"errors"
	"fmt"

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
