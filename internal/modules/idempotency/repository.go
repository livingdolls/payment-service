package idempotency

import (
	"context"
	"time"
)

type Repository interface {
	Acquire(ctx context.Context, operation string, key string, requestHash string, expiresAt time.Time) (record *Record, created bool, err error)

	Complete(ctx context.Context, id string, responseStatus int, responseBody []byte) error
}
