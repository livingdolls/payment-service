-- +goose Up

ALTER TABLE payment_attempts
ADD COLUMN reconcile_state VARCHAR(20)
    NOT NULL DEFAULT 'ACTIVE',
ADD COLUMN reconcile_failures INTEGER
    NOT NULL DEFAULT 0;

ALTER TABLE payment_attempts
ADD CONSTRAINT chk_reconcile_state
CHECK (reconcile_state IN ('ACTIVE', 'NEEDS_REVIEW'));

ALTER TABLE payment_attempts
ADD CONSTRAINT chk_reconcile_failures
CHECK (reconcile_failures >= 0);

DROP INDEX IF EXISTS idx_payment_attempts_reconcile;

CREATE INDEX idx_payment_attempts_reconcile
ON payment_attempts (next_reconcile_at, updated_at)
WHERE provider = 'XENDIT'
    AND reconcile_state = 'ACTIVE'
    AND status IN (
      'UNKNOWN',
      'REQUESTING_PROVIDER',
      'PENDING',
      'REQUIRES_ACTION',
      'AUTHORIZED'
    );

-- +goose Down

DROP INDEX IF EXISTS idx_payment_attempts_reconcile;

ALTER TABLE payment_attempts
DROP CONSTRAINT chk_reconcile_failures,
DROP CONSTRAINT chk_reconcile_state;

ALTER TABLE payment_attempts
DROP COLUMN reconcile_failures,
DROP COLUMN reconcile_state;

CREATE INDEX idx_payment_attempts_reconcile
ON payment_attempts (next_reconcile_at, updated_at)
WHERE provider = 'XENDIT'
    AND provider_payment_request_id IS NOT NULL
    AND status IN (
      'UNKNOWN',
      'REQUESTING_PROVIDER',
      'PENDING',
      'REQUIRES_ACTION',
      'AUTHORIZED'
    );