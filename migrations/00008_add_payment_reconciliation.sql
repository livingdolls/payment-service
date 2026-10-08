-- +goose Up

ALTER TABLE payment_attempts
ADD COLUMN next_reconcile_at TIMESTAMPTZ,
ADD COLUMN reconcile_attempts INTEGER NOT NULL DEFAULT 0,
ADD COLUMN last_reconcile_error TEXT;

CREATE INDEX idx_payment_attempts_reconcile
ON payment_attempts (
    next_reconcile_at,
    updated_at
)
WHERE
    provider = 'XENDIT'
    AND provider_payment_request_id IS NOT NULL
    AND status IN (
        'UNKNOWN',
        'REQUESTING_PROVIDER',
        'PENDING',
        'REQUIRES_ACTION',
        'AUTHORIZED'
    );

-- +goose Down

DROP INDEX idx_payment_attempts_reconcile;

ALTER TABLE payment_attempts
DROP COLUMN last_reconcile_error,
DROP COLUMN reconcile_attempts,
DROP COLUMN next_reconcile_at;