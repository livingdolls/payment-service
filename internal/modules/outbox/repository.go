package outbox

import (
	"context"
	"errors"
	"time"
)

var (
	ErrClaimLost      = errors.New("outbox claim is no longer owned by this worker")
	ErrInvalidPayload = errors.New("invalid or unsupported outbox payload")
)

type Repository interface {
	ClaimNext(ctx context.Context, lease time.Duration) (*Event, error)
	MarkPublished(ctx context.Context, id string, claimToken string) error
	MarkForRetry(ctx context.Context, id string, claimToken string, maxAttempts int, delay time.Duration, message string) (Status, error)
	MarkDead(ctx context.Context, id string, claimToken string, message string) error
}
