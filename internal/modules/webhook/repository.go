package webhook

import (
	"context"
	"time"
)

type Repository interface {
	Store(ctx context.Context, event *Event) (bool, error)
	ClaimNext(ctx context.Context, staleAfter time.Duration) (*Event, error)
	MarkProcessed(ctx context.Context, id string) error
	MarkForRetry(ctx context.Context, id string, message string, maxAttempts int) error
}
