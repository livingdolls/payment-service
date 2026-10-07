-- +goose Up

CREATE TABLE webhook_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    provider VARCHAR(32) NOT NULL,

    event_key VARCHAR(255) NOT NULL,
    event_type VARCHAR(100) NOT NULL,

    provider_payment_id VARCHAR(255),
    provider_payment_request_id VARCHAR(255),
    reference_id VARCHAR(255),

    payload JSONB NOT NULL,

    status VARCHAR(16) NOT NULL DEFAULT 'RECEIVED',

    processing_attempts INTEGER NOT NULL DEFAULT 0,

    error_message TEXT,
    
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,

    CONSTRAINT uq_webhook_events_provider_event
        UNIQUE (
            provider,
            event_key
        ),

    CONSTRAINT chk_webhook_events_status
        CHECK (
            status IN (
                'RECEIVED',
                'PROCESSING',
                'PROCESSED',
                'FAILED'
            )
        ),

    CONSTRAINT chk_webhook_events_processing_attempts
        CHECK (
            processing_attempts >= 0
        )
);

CREATE INDEX idx_webhook_events_status
    ON webhook_events (status);

CREATE INDEX idx_webhook_events_provider_payment_id
    ON webhook_events (provider_payment_id);

CREATE INDEX idx_webhook_events_payment_request_id
    ON webhook_events (provider_payment_request_id);

CREATE INDEX idx_webhook_events_received_at
    ON webhook_events (received_at);

-- +goose Down

DROP TABLE webhook_events;