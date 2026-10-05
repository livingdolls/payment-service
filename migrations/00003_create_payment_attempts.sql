-- +goose Up

CREATE TABLE payment_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_intent_id UUID NOT NULL,
    attempt_number INTEGER NOT NULL,
    provider VARCHAR(32) NOT NULL,
    provider_idempotency_key VARCHAR(255) NOT NULL,
    provider_payment_request_id VARCHAR(255),
    provider_payment_id VARCHAR(255),
    status VARCHAR(32) NOT NULL DEFAULT 'CREATED',
    error_code VARCHAR(100),
    error_message TEXT,
    provider_response JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_payment_attempts_payment_intent
        FOREIGN KEY (payment_intent_id)
        REFERENCES payment_intents(id)
        ON DELETE RESTRICT,

    CONSTRAINT uq_payment_attempts_number
        UNIQUE (
            payment_intent_id,
            attempt_number
        ),

    CONSTRAINT uq_payment_attempts_provider_idempotency_key
        UNIQUE (
            provider,
            provider_idempotency_key
        ),

    CONSTRAINT chk_payment_attempts_attempt_number
        CHECK (attempt_number > 0),

    CONSTRAINT chk_payment_attempts_status
        CHECK (
            status IN (
                'CREATED',
                'REQUESTING_PROVIDER',
                'REQUIRES_ACTION',
                'AUTHORIZED',
                'CAPTURED',
                'FAILED',
                'EXPIRED',
                'UNKNOWN'
            )
        )
);

CREATE INDEX idx_payment_attempts_payment_intent_id
    ON payment_attempts (payment_intent_id);

CREATE INDEX idx_payment_attempts_status
    ON payment_attempts (status);

CREATE INDEX idx_payment_attempts_provider_payment_request_id
    ON payment_attempts (provider_payment_request_id);

CREATE INDEX idx_payment_attempts_provider_payment_id
    ON payment_attempts (provider_payment_id);

-- +goose Down

DROP TABLE payment_attempts;