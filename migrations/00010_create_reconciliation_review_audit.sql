-- +goose Up

ALTER TABLE payment_attempts
ADD COLUMN reconcile_force BOOLEAN NOT NULL DEFAULT FALSE,
ADD COLUMN reconcile_generation BIGINT NOT NULL DEFAULT 0;

ALTER TABLE payment_attempts
ADD CONSTRAINT chk_reconcile_generation
CHECK (reconcile_generation >= 0);

CREATE TABLE payment_reconciliation_actions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_attempt_id UUID NOT NULL REFERENCES payment_attempts(id),
    actor VARCHAR(100) NOT NULL,
    action VARCHAR(40) NOT NULL,
    reason TEXT NOT NULL,
    previous_state VARCHAR(20) NOT NULL,
    new_state VARCHAR(20) NOT NULL,
    previous_failures INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_reconciliation_action
    CHECK (
        action IN ('REQUEUE')
    ),

    CONSTRAINT chk_reconciliation_reason
    CHECK (
        char_length(trim(reason)) BETWEEN 10 AND 500
    )
);

CREATE INDEX idx_reconciliation_actions_attempt
ON payment_reconciliation_actions (
    payment_attempt_id,
    created_at DESC
);

-- +goose Down

DROP TABLE payment_reconciliation_actions;

ALTER TABLE payment_attempts
DROP CONSTRAINT chk_reconcile_generation;

ALTER TABLE payment_attempts
DROP COLUMN reconcile_force,
DROP COLUMN reconcile_generation;