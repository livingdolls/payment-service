-- +goose Up

CREATE TABLE orders (
    id VARCHAR(100) PRIMARY KEY,

    amount BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL,

    status VARCHAR(24) NOT NULL DEFAULT 'PENDING_PAYMENT',

    payment_intent_id UUID UNIQUE,

    paid_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_orders_amount CHECK (amount > 0),

    CONSTRAINT chk_orders_status 
        CHECK (
            status IN (
                'PENDING_PAYMENT',
                'PAID',
                'CANCELED'
            )
        ),

    CONSTRAINT chk_orders_paid
        CHECK (
            status <> 'PAID'
            OR (
                payment_intent_id IS NOT NULL
                AND paid_at IS NOT NULL
            )
        )
);

CREATE TABLE consumed_payment_events (
    event_id UUID PRIMARY KEY,
    event_type VARCHAR(100) NOT NULL,
    order_id VARCHAR(100) NOT NULL,
    payment_intent_id UUID NOT NULL,
    payload_hash CHAR(64) NOT NULL,
    consumed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_consumed_payment_events_order
ON consumed_payment_events (order_id);

-- +goose Down

DROP TABLE consumed_payment_events;
DROP TABLE orders;