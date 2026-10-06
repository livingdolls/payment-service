-- +goose Up

ALTER TABLE payment_attempts
ADD COLUMN version BIGINT NOT NULL DEFAULT 1;

ALTER TABLE payment_attempts
ADD CONSTRAINT chk_payment_attempts_version
CHECK (version > 0);

-- +goose Down
ALTER TABLE payment_attempts
DROP CONSTRAINT chk_payment_attempts_version;

ALTER TABLE payment_attempts
DROP COLUMN version;