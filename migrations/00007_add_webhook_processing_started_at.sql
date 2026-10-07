-- +goose Up

ALTER TABLE webhook_events
ADD COLUMN processing_started_at TIMESTAMPTZ;

-- +goose Down

ALTER TABLE webhook_events
DROP COLUMN processing_started_at;