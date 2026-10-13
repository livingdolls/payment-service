-- +goose Up

CREATE UNIQUE INDEX uq_payment_intents_order_id
ON payment_intents (order_id);

-- +goose Down

DROP INDEX uq_payment_intents_order_id;