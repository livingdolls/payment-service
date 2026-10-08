package application

import (
	"context"
	"fmt"
	"time"

	"github.com/livingdolls/payment-service/internal/database"
	"github.com/livingdolls/payment-service/internal/modules/payment"
	paymentpostgres "github.com/livingdolls/payment-service/internal/modules/payment/postgres"
	"github.com/livingdolls/payment-service/internal/provider"
)

const (
	reconcileMinAge   = 2 * time.Minute
	reconcileCooldown = 5 * time.Minute
)

type ReconciliationProcessor struct {
	db             database.DBTX
	statusReader   provider.PaymentStatusReader
	processPayment *ProcessPaymentUseCase
}

func NewReconciliationProcessor(db database.DBTX, statusReader provider.PaymentStatusReader, processPayment *ProcessPaymentUseCase) *ReconciliationProcessor {
	return &ReconciliationProcessor{
		db:             db,
		statusReader:   statusReader,
		processPayment: processPayment,
	}
}

func (p *ReconciliationProcessor) ProcessNext(ctx context.Context) (bool, error) {
	repo := paymentpostgres.NewReconciliationRepository(p.db)

	attempt, err := repo.ClaimNext(
		ctx,
		reconcileMinAge,
		reconcileCooldown,
	)

	if err != nil {
		return false, err
	}

	if attempt == nil {
		return false, nil
	}

	err = p.reconcileAttempt(
		ctx,
		attempt.ID,
	)

	if err != nil {
		message := err.Error()

		if len(message) > 500 {
			message = message[:500]
		}

		recordErr := repo.RecordError(
			ctx,
			attempt.ID,
			message,
		)

		if recordErr != nil {
			return true, fmt.Errorf(
				"reconciliation failed: %v; record error: %w",
				err,
				recordErr,
			)
		}

		return true, err
	}

	return true, nil

}

func (p *ReconciliationProcessor) reconcileAttempt(ctx context.Context, attemptID string) error {
	attemptRepository := paymentpostgres.NewAttemptRepository(p.db)
	paymentRepository := paymentpostgres.NewRepository(p.db)

	attempt, err := attemptRepository.GetAttemptByID(
		ctx,
		attemptID,
	)

	if err != nil {
		return err
	}

	// Webhook bisa saja sudah menyelesaikan payment
	// setelah attempt di-claim.

	if isTerminalAttempt(attempt.Status) {
		return nil
	}

	intent, err := paymentRepository.GetByID(
		ctx,
		attempt.PaymentIntentID,
	)

	if err != nil {
		return err
	}

	if attempt.ProviderPaymentRequestID == nil {
		return fmt.Errorf(
			"cannot reconcile without provider payment request id",
		)
	}

	paymentRequestID := *attempt.ProviderPaymentRequestID

	// Tidak ada transaction DB yang terbuka
	// saat request ke Xendit dilakukan.
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)

	snapshot, err := p.statusReader.GetPaymentRequest(
		requestCtx,
		paymentRequestID,
	)

	cancel()

	if err != nil {
		return fmt.Errorf(
			"get provider payment request: %w",
			err,
		)
	}

	// Pastikan hasil dari Xendit benar-benar
	// milik PaymentIntent yang kita cari.

	if snapshot.PaymentRequestID != paymentRequestID {
		return fmt.Errorf(
			"provider payment request id missmatch",
		)
	}

	if snapshot.ReferenceID != intent.ReferenceID {
		return fmt.Errorf(
			"provider reference id mismatch",
		)
	}

	if snapshot.Currency != intent.Currency {
		return fmt.Errorf(
			"provider currency mismatch",
		)
	}

	if snapshot.Amount != intent.Amount {
		return fmt.Errorf("provider amount mismatch")
	}

	// Reuse state-transition logic dari
	// ProcessPaymentUseCase.

	_, err = p.processPayment.applyProviderResult(
		ctx,
		attemptID,
		&provider.CreatePaymentResult{
			PaymentRequestID: snapshot.PaymentRequestID,
			PaymentID:        snapshot.PaymentID,
			Status:           snapshot.Status,
			Actions:          snapshot.Actions,
			RawResponse:      snapshot.RawResponse,
		},
	)

	if err != nil {
		return fmt.Errorf("apply reconciled payment state: %w", err)
	}

	return nil
}

func isTerminalAttempt(status payment.AttemptStatus) bool {
	switch status {
	case payment.AttemptStatusCaptured, payment.AttemptStatusFailed, payment.AttemptStatusExpired, payment.AttemptStatusCanceled:
		return true

	default:
		return true
	}
}
