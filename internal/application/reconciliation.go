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
	db              database.DBTX
	statusReader    provider.PaymentStatusReader
	referenceLookup provider.TransactionReferenceLookup
	processPayment  *ProcessPaymentUseCase
}

func NewReconciliationProcessor(db database.DBTX, statusReader provider.PaymentStatusReader, referenceLookup provider.TransactionReferenceLookup, processPayment *ProcessPaymentUseCase) *ReconciliationProcessor {
	return &ReconciliationProcessor{
		db:              db,
		statusReader:    statusReader,
		referenceLookup: referenceLookup,
		processPayment:  processPayment,
	}
}

func (p *ReconciliationProcessor) ProcessNext(ctx context.Context) (bool, error) {
	repo := paymentpostgres.NewReconciliationRepository(p.db)

	job, err := repo.ClaimNext(ctx, reconcileMinAge, reconcileCooldown)

	if err != nil {
		return false, err
	}

	if job == nil {
		return false, nil
	}

	err = p.reconcileAttempt(
		ctx,
		job.AttemptID,
	)

	if err != nil {
		nextFailure := job.ReconcileFailures + 1

		delay := reconciliationBackoff(nextFailure)

		markErr := repo.MarkFailure(
			ctx,
			job,
			err.Error(),
			delay,
			reconcileMaxFailures,
		)

		if markErr != nil {
			return true, fmt.Errorf(
				"reconcile: %v; mark failure: %w",
				err,
				markErr,
			)
		}

		return true, err
	}

	if err := repo.MarkSuccess(
		ctx,
		job,
		reconcileCooldown,
	); err != nil {
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

	var paymentRequestID string

	if attempt.ProviderPaymentRequestID != nil {
		paymentRequestID = *attempt.ProviderPaymentRequestID
	}

	// Recovery khusus jika Xendit belum
	// memberikan payment_request_id kepada kita.

	if paymentRequestID == "" {
		lookupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)

		recoverID, found, lookupErr := p.referenceLookup.FindPaymentRequestIDByReference(
			lookupCtx,
			intent.ReferenceID,
			intent.Currency,
			intent.Amount,
		)

		cancel()

		if lookupErr != nil {
			return fmt.Errorf(
				"lookup transaction by reference: %w",
				lookupErr,
			)
		}

		if !found {
			return fmt.Errorf(
				"provider payment request not yet identifable for reference %s",
				intent.ReferenceID,
			)
		}

		paymentRequestID = recoverID
	}

	requestCtx, cancel := context.WithTimeout(
		ctx,
		10*time.Second,
	)

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

	// Wajib divalidasi sebelum update state.

	if snapshot.PaymentRequestID != paymentRequestID {
		return fmt.Errorf("provider payment request ID mismatch")
	}

	if snapshot.ReferenceID != intent.ReferenceID {
		return fmt.Errorf("provieder reference ID mismatch")
	}

	if snapshot.Amount != intent.Amount {
		return fmt.Errorf(
			"provider payment amount mismatch",
		)
	}

	if snapshot.Currency != intent.Currency {
		return fmt.Errorf(
			"provider payment currency mismatch",
		)
	}

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
		return fmt.Errorf("apply reconciled state: %w", err)
	}

	return nil
}

func isTerminalAttempt(status payment.AttemptStatus) bool {
	switch status {
	case payment.AttemptStatusCaptured,
		payment.AttemptStatusFailed,
		payment.AttemptStatusExpired,
		payment.AttemptStatusCanceled:
		return true

	default:
		return false
	}
}
