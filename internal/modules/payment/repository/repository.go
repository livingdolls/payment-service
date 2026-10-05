package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/livingdolls/payment-service/internal/modules/payment"
)

type Repository struct {
	db *pgxpool.Pool
}

var _ payment.Repository = (*Repository)(nil)

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db: db,
	}
}

func (r *Repository) Create(ctx context.Context, p *payment.PaymentIntent) error {
	const query = `
	INSERT INTO payment_intents (
		reference_id,
		order_id,
		amount,
		currency,
		status,
		capture_method
	)
	VALUES (
		$1, $2, $3, $4, $5, $6
	)
	RETURNING id, captured_amount, refunded_amount, version, created_at, updated_at
	`

	err := r.db.QueryRow(
		ctx,
		query,
		p.ReferenceID,
		p.OrderID,
		p.Amount,
		p.Currency,
		p.Status,
		p.CaptureMethod,
	).Scan(
		&p.ID,
		&p.CapturedAmount,
		&p.RefundedAmount,
		&p.Version,
		&p.CreatedAt,
		&p.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("create payment intent: %w", err)
	}

	return nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (*payment.PaymentIntent, error) {
	const query = `
		SELECT
			id,
			reference_id,
			order_id,
			amount,
			currency,
			status,
			capture_method,
			captured_amount,
			refunded_amount,
			version,
			created_at,
			updated_at
		FROM payment_intents
		WHERE id = $1
	`

	var p payment.PaymentIntent

	err := r.db.QueryRow(ctx, query, id).Scan(
		&p.ID,
		&p.ReferenceID,
		&p.OrderID,
		&p.Amount,
		&p.Currency,
		&p.Status,
		&p.CaptureMethod,
		&p.CapturedAmount,
		&p.RefundedAmount,
		&p.Version,
		&p.CreatedAt,
		&p.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("get payment intent by id: %w", err)
	}

	return &p, nil
}
