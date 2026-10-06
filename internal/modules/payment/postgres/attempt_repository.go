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
		RETURNIN
			id,
			created_at
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
	panic("unimplemented")
}

// GetLatestAttempt implements [payment.AttemptRepository].
func (a *AttemptRepository) GetLatestAttempt(ctx context.Context, paymentIntentId string) (*payment.PaymentAttempt, error) {
	panic("unimplemented")
}
