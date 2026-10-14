-- +goose Up

ALTER TABLE orders
ADD COLUMN customer_id VARCHAR(100);

CREATE INDEX idx_orders_customer
ON orders (customer_id, created_at DESC);

-- +goose Down

DROP INDEX idx_orders_customer;

ALTER TABLE orders
DROP COLUMN customer_id;