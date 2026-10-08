package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/livingdolls/payment-service/internal/application"
)

type ReconciliationWorker struct {
	processor    *application.ReconciliationProcessor
	pollInterval time.Duration
}

func NewReconciliationWorker(processor *application.ReconciliationProcessor) *ReconciliationWorker {
	return &ReconciliationWorker{
		processor:    processor,
		pollInterval: 30 * time.Second,
	}
}

func (w *ReconciliationWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("reconciliation worker stopped")
			return

		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

func (w *ReconciliationWorker) processBatch(ctx context.Context) {
	const maxBatchSize = 10

	for i := 0; i < maxBatchSize; i++ {
		if ctx.Err() != nil {
			return
		}

		processed, err := w.processor.ProcessNext(ctx)

		if err != nil {
			slog.Error(
				"payment reconciliation failed",
				"error",
				err,
			)
		}

		if !processed {
			return
		}
	}
}
