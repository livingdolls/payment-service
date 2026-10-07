package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/livingdolls/payment-service/internal/application"
)

type WebhookWorker struct {
	processor    *application.WebhookProcessor
	poolInterval time.Duration
}

func NewWebhookWorker(processor *application.WebhookProcessor) *WebhookWorker {
	return &WebhookWorker{
		processor:    processor,
		poolInterval: 1 * time.Second,
	}
}

func (w *WebhookWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.poolInterval)

	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("webhook worker stopped")

			return

		case <-ticker.C:
			w.processAvailable(ctx)
		}
	}
}

func (w *WebhookWorker) processAvailable(ctx context.Context) {
	for {
		processed, err := w.processor.ProcessNext(ctx)

		if err != nil {
			if !errors.Is(err, context.Canceled) {
				slog.Error("webhook processing failed", "error", err)
			}

			return
		}

		if !processed {
			return
		}
	}
}
