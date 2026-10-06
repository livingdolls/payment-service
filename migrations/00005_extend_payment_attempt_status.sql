-- +goose Up

ALTER TABLE payment_attempts
ADD CONSTRAINT chk_payment_attempts_status
CHECK (
    status IN (
        'CREATED',
        'REQUESTING_PROVIDER',
        'PENDING',
        'REQUIRES_ACTION',
        'AUTHORIZED',
        'CAPTURED',
        'FAILED',
        'EXPIRED',
        'CANCELED',
        'UNKNOWN'
    )
);

ALTER TABLE payment_attempts
ADD COLUMN provider_request JSONB;


-- +goose Down

ALTER TABLE payment_attempts
DROP COLUMN provider_request;

ALTER TABLE payment_attempts
DROP CONSTRAINT chk_payment_attempts_status;