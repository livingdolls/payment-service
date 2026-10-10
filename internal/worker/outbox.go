package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/livingdolls/payment-service/internal/modules/outbox"
)

const (
	outboxMaxAttempts  = 8
	outboxLease        = 2 * time.Minute
	outboxPollInterval = 1 * time.Second
)

type OutboxWorker struct {
	repository outbox.Repository
	publisher  outbox.Publisher
}

func NewOutboxWorker(repository outbox.Repository, publisher outbox.Publisher) *OutboxWorker {
	return &OutboxWorker{
		repository: repository,
		publisher:  publisher,
	}
}

func (w *OutboxWorker) ProcessNext(ctx context.Context) (bool, error) {
	event, err := w.repository.ClaimNext(ctx, outboxLease)
	if err != nil {
		return false, err
	}

	if event == nil {
		return false, nil
	}

	if event.ClaimToken == nil {
		return true, fmt.Errorf("claimed outbox event has no claim token")
	}

	token := *event.ClaimToken

	// jika worker sebelumnya crash pada percobaan terakhir
	// jangan melakukan delivery tambahan
	if event.AttemptCount > outboxMaxAttempts {
		err := w.repository.MarkDead(
			ctx,
			event.ID,
			token,
			"maximun delivery attempts exceeded",
		)

		return true, err
	}

	err = w.publisher.Publish(ctx, event)

	if err == nil {
		if err := w.repository.MarkPublished(
			ctx,
			event.ID,
			token,
		); err != nil {
			return true, fmt.Errorf("mark published: %w", err)
		}

		slog.Info(
			"outbox event published",
			"event", event.ID,
			"event_type", event.EventType,
		)

		return true, nil
	}

	// payload rusak / unsupported tidak perlu di kirim berulang
	if errors.Is(err, outbox.ErrInvalidPayload) {
		markErr := w.repository.MarkDead(
			ctx,
			event.ID,
			token,
			err.Error(),
		)

		if markErr != nil {
			return true, fmt.Errorf("publish error %v; mark dead: %w", err, markErr)
		}

		return true, nil
	}

	status, markErr := w.repository.MarkForRetry(
		ctx,
		event.ID,
		token,
		outboxMaxAttempts,
		outboxBackoff(event.AttemptCount),
		err.Error(),
	)

	if markErr != nil {
		return true, fmt.Errorf("publish error %v; mark retry: %w", markErr, err)
	}

	slog.Warn(
		"outbox event published",
		"event", event.ID,
		"attempt_count", event.AttemptCount,
		"next_status", status,
		"error", err)

	return true, nil
}

func outboxBackoff(attempts int) time.Duration {
	delay := 10 * time.Second

	const maxDelay = 15 * time.Minute

	for range attempts {
		if delay >= maxDelay/2 {
			return maxDelay
		}

		delay *= 2
	}

	return delay
}

func (w *OutboxWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(outboxPollInterval)

	defer ticker.Stop()

	slog.Info("outbox worker started")

	for {
		select {
		case <-ctx.Done():
			slog.Info("outbox worker shutting down")
		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

func (w *OutboxWorker) processBatch(ctx context.Context) {
	const batchSize = 20

	for range batchSize {
		if ctx.Err() != nil {
			return
		}

		processed, err := w.ProcessNext(ctx)

		if err != nil {
			slog.Error("outbox worker failed to process batch", "err", err)
		}

		if !processed {
			return
		}
	}
}
