-- +goose Up

CREATE TABLE idempotency_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    operation VARCHAR(100) NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL,
    request_hash CHAR(64) NOT NULL,

    status VARCHAR(16) NOT NULL DEFAULT 'PROCESSING',

    response_status INTEGER,
    response_body JSONB,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT uq_idempotency_keys_operation_key 
        UNIQUE (operation, idempotency_key),

    CONSTRAINT chk_idempotency_keys_status
        CHECK (
            status IN (
                'PROCESSING',
                'COMPLETED'
            )
        ),

    CONSTRAINT chk_idempotency_keys_response_status 
        CHECK (
            response_status IS NULL
            OR (
                response_status >= 100
                AND response_status <= 599
            )
        ),

    CONSTRAINT chk_idempotency_keys_completed_response
        CHECK (
            status != 'COMPLETED'
            OR (
                response_status IS NOT NULL
                AND response_body IS NOT NULL
            )
        )
);

CREATE INDEX idx_idempotency_keys_expires_at
    ON idempotency_keys (expires_at);

CREATE INDEX idx_idempotency_keys_status
    ON idempotency_keys (status);

-- +goose Down

DROP TABLE idempotency_keys;