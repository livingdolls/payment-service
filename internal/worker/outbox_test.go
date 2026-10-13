package worker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/livingdolls/payment-service/internal/modules/outbox"
)

func TestOutboxWorkerRun_CanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	worker := NewOutboxWorker(outboxRepositoryStub{}, outboxPublisherStub{})

	var runErr error
	done := make(chan struct{})
	go func() {
		runErr = worker.Run(ctx)
		close(done)
	}()

	timer := time.NewTimer(time.Second)
	defer timer.Stop()

	select {
	case <-done:
		if runErr != nil {
			t.Fatalf("Run returned an error on cancellation: %v", runErr)
		}
	case <-timer.C:
		t.Fatal("Run did not stop promptly after context cancellation")
	}
}

func TestOutboxWorkerProcessNext_RetryFailure(t *testing.T) {
	t.Parallel()

	publishErr := errors.New("order service unavailable")
	retryErr := errors.New("retry persistence unavailable")
	claimToken := "claim-token"
	repository := outboxRepositoryStub{
		event: &outbox.Event{
			ID:           "event-1",
			AttemptCount: 1,
			ClaimToken:   &claimToken,
		},
		retryErr: retryErr,
	}
	worker := NewOutboxWorker(repository, outboxPublisherStub{err: publishErr})

	processed, err := worker.ProcessNext(t.Context())
	if !processed {
		t.Fatal("claimed event was not reported as processed")
	}
	if !errors.Is(err, retryErr) {
		t.Fatalf("ProcessNext error = %v, want wrapped retry failure", err)
	}
	if !strings.Contains(err.Error(), publishErr.Error()) {
		t.Fatalf("ProcessNext error lost the publish failure: %v", err)
	}
}

type outboxRepositoryStub struct {
	event    *outbox.Event
	retryErr error
}

func (r outboxRepositoryStub) ClaimNext(context.Context, time.Duration) (*outbox.Event, error) {
	return r.event, nil
}

func (r outboxRepositoryStub) MarkPublished(context.Context, string, string) error {
	return errors.New("unexpected mark published")
}

func (r outboxRepositoryStub) MarkForRetry(
	context.Context,
	string,
	string,
	int,
	time.Duration,
	string,
) (outbox.Status, error) {
	return "", r.retryErr
}

func (r outboxRepositoryStub) MarkDead(context.Context, string, string, string) error {
	return errors.New("unexpected mark dead")
}

type outboxPublisherStub struct {
	err error
}

func (p outboxPublisherStub) Publish(context.Context, *outbox.Event) error {
	return p.err
}
