-- +goose Up

CREATE TABLE outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_key VARCHAR(200) NOT NULL UNIQUE,
    aggregate_type VARCHAR(50) NOT NULL,
    aggregate_id UUID NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'PENDING',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claim_token UUID,
    claimed_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_outbox_status
        CHECK (
            status IN (
                'PENDING',
                'PROCESSING',
                'PUBLISHED',
                'DEAD'
            )
        ),

    CONSTRAINT chk_outbox_attempt_count
        CHECK (attempt_count >= 0)
);

CREATE INDEX idx_outbox_pending
ON outbox_events (available_at, created_at)
WHERE status = 'PENDING';

CREATE INDEX idx_outbox_processing
ON outbox_events (claimed_at)
WHERE status = 'PROCESSING';

ALTER TABLE payment_intents
ADD CONSTRAINT chk_payment_intents_full_capture
CHECK (
    status <> 'CAPTURED'
    OR captured_amount = amount
);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION enqueue_payment_captured_outbox()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    new_event_id UUID := gen_random_uuid();
BEGIN
    IF NEW.status = 'CAPTURED'
        AND OLD.status IS DISTINCT FROM NEW.status
    THEN
        INSERT INTO outbox_events (
            id,
            event_key,
            aggregate_type,
            aggregate_id,
            event_type,
            payload
        )
        VALUES (
            new_event_id,
            'payment_intent:' || NEW.id::TEXT || ':captured:v1',
            'PAYMENT_INTENT',
            NEW.id,
            'payment.captured.v1',
            jsonb_build_object(
                'event_id', new_event_id::TEXT,
                'event_type', 'payment.capture.v1',
                'payment_intent_id', NEW.id::TEXT,
                'reference_id', NEW.reference_id,
                'order_id', NEW.order_id,
                'amount', NEW.amount,
                'currency', NEW.currency,
                'captured_amount', NEW.captured_amount,
                'payment_version', NEW.version,
                'occurred_at', NEW.updated_at
            )
        )
        ON CONFLICT (event_key) DO NOTHING;

    END IF;

    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_payment_intent_captured_outbox
AFTER UPDATE OF status
ON payment_intents
FOR EACH ROW
EXECUTE FUNCTION enqueue_payment_captured_outbox();

-- +goose Down

DROP TRIGGER IF EXISTS
    trg_payment_intent_captured_outbox
ON payment_intents;

DROP FUNCTION IF EXISTS
    enqueue_payment_captured_outbox();

ALTER TABLE payment_intents
DROP CONSTRAINT IF EXISTS
    chk_payment_intents_full_capture;

DROP TABLE IF EXISTS outbox_events;