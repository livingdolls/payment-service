package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/livingdolls/payment-service/internal/database"
	"github.com/livingdolls/payment-service/internal/modules/idempotency"
)

type Repository struct {
	db database.DBTX
}

var _ idempotency.Repository = (*Repository)(nil)

func NewRepository(db database.DBTX) *Repository {
	return &Repository{
		db: db,
	}
}

// Acquire implements [idempotency.Repository].
func (r *Repository) Acquire(ctx context.Context, operation string, key string, requestHash string, expiresAt time.Time) (record *idempotency.Record, created bool, err error) {
	const insertQuery = `
		INSERT INTO idempotency_keys (
			operation,
			idempotency_key,
			request_hash,
			status,
			expires_at
		)
		VALUES (
			$1, $2, $3, $4, $5
		)
		ON CONFLICT (
			operation,
			idempotency_key
		)
		DO NOTHING
		RETURNING
			id,
			operation,
			idempotency_key,
			request_hash,
			status,
			response_status,
			response_body,
			created_at,
			updated_at,
			expires_at
	`

	idemRecord := &idempotency.Record{}

	errIdem := r.db.QueryRow(
		ctx,
		insertQuery,
		operation,
		key,
		requestHash,
		idempotency.StatusProcessing,
		expiresAt,
	).Scan(
		&idemRecord.ID,
		&idemRecord.Operation,
		&idemRecord.Key,
		&idemRecord.RequestHash,
		&idemRecord.Status,
		&idemRecord.ResponseStatus,
		&idemRecord.ResponseBody,
		&idemRecord.CreatedAt,
		&idemRecord.UpdatedAt,
	)

	if errIdem == nil {
		return idemRecord, true, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("insert idempotency key: %w", err)
	}

	return r.getByOperationAndKey(ctx, operation, key)
}

// Complete implements [idempotency.Repository].
func (r *Repository) Complete(ctx context.Context, id string, responseStatus int, responseBody []byte) error {
	const query = `
		UPDATE idempotency_keys
		SET
			status = $1,
			response_status = $2,
			response_body = $3::jsonb,
			updated_at = NOW()
		WHERE id = $4
			AND status = $5
	`

	result, err := r.db.Exec(
		ctx,
		query,
		idempotency.StatusCompleted,
		responseStatus,
		responseBody,
		id,
		idempotency.StatusProcessing,
	)

	if err != nil {
		return fmt.Errorf("complete idempotency key: %w", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("complete idempotency key: record is not procession")
	}

	return nil
}

func (r *Repository) getByOperationAndKey(
	ctx context.Context,
	operation string,
	key string,
) (*idempotency.Record, bool, error) {
	const query = `
		SELECT
			id,
			operation,
			idempotency_key,
			request_hash,
			status,
			response_status,
			response_body,
			created_at,
			updated_at,
			expires_at
		FROM idempotency_keys
		WHERE operation = $1
			AND idempotency_key = $2
	`

	record := &idempotency.Record{}

	err := r.db.QueryRow(ctx, query, operation, key).Scan(
		&record.ID,
		&record.Operation,
		&record.Key,
		&record.RequestHash,
		&record.Status,
		&record.ResponseStatus,
		&record.ResponseBody,
		&record.CreatedAt,
		&record.UpdatedAt,
		&record.ExpiresAt,
	)

	if err != nil {
		return nil, false, fmt.Errorf("get idempotency key: %w", err)
	}

	return record, false, nil
}
