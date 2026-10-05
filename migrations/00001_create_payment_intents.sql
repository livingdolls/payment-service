-- +goose Up

CREATE TABLE payment_intents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    reference_id VARCHAR(100) NOT NULL,
    order_id VARCHAR(100) NOT NULL,

    amount BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL,

    status VARCHAR(32) NOT NULL,
    capture_method VARCHAR(16) NOT NULL DEFAULT 'AUTOMATIC',

    captured_amount BIGINT NOT NULL DEFAULT 0,
    refunded_amount BIGINT NOT NULL DEFAULT 0,

    version BIGINT NOT NULL DEFAULT 1,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_payment_intents_reference_id UNIQUE (reference_id),

    CONSTRAINT chk_payment_intents_amount CHECK (amount > 0),

    CONSTRAINT chk_payment_intents_currency CHECK (CHAR_LENGTH(currency) = 3 AND currency = UPPER(currency)),

    CONSTRAINT chk_payment_intents_status CHECK (
        status IN (
            'CREATED',
            'PROCESSING',
                'REQUIRES_ACTION',
                'AUTHORIZED',
                'CAPTURED',
                'PARTIALLY_REFUNDED',
                'REFUNDED',
                'FAILED',
                'CANCELED',
                'EXPIRED'
        )
    ),

    CONSTRAINT chk_payment_intents_capture_method CHECK (
        capture_method IN (
            'AUTOMATIC',
            'MANUAL'
        )
    ),

    CONSTRAINT chk_payment_intents_captured_amount CHECK (
        captured_amount >= 0 AND captured_amount <= amount
    ),

    CONSTRAINT chk_payment_intents_refunded_amount CHECK (
        refunded_amount >= 0 AND refunded_amount <= captured_amount
    ),

    CONSTRAINT chk_payment_intents_version CHECK (version > 0)
);

CREATE INDEX idx_payment_intents_order_id ON payment_intents (order_id);
CREATE INDEX idx_payment_intents_status ON payment_intents (status);
CREATE INDEX idx_payment_intents_created_at ON payment_intents (created_at);

-- +goose Down
DROP TABLE payment_intents;