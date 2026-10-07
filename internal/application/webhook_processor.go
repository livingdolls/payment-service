package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/livingdolls/payment-service/internal/database"
	"github.com/livingdolls/payment-service/internal/modules/payment"
	paymentpostgres "github.com/livingdolls/payment-service/internal/modules/payment/postgres"
	"github.com/livingdolls/payment-service/internal/modules/webhook"
	webhookpostgres "github.com/livingdolls/payment-service/internal/modules/webhook/postgres"
	xenditprovider "github.com/livingdolls/payment-service/internal/provider/xendit"
)

const (
	webhookStaleAfter  = 5 * time.Minute
	webhookMaxAttempts = 5
)

type WebhookProcessor struct {
	db                 database.DBTX
	transactionManager database.TransactionManager
}

func NewWebhookProcessor(db database.DBTX, transactionManager database.TransactionManager) *WebhookProcessor {
	return &WebhookProcessor{
		db:                 db,
		transactionManager: transactionManager,
	}
}

func (p *WebhookProcessor) ProcessNext(ctx context.Context) (bool, error) {
	repository := webhookpostgres.NewRepository(p.db)

	event, err := repository.ClaimNext(ctx, webhookStaleAfter)

	if errors.Is(err, webhook.ErrNoEventAvailable) {
		return false, nil
	}

	if err != nil {
		return false, nil
	}

	if err := p.processEvent(ctx, event); err != nil {
		markErr := repository.MarkForRetry(
			ctx,
			event.ID,
			err.Error(),
			webhookMaxAttempts,
		)

		if markErr != nil {
			return true, fmt.Errorf("process webhook: %v; mark retry: %w", err, markErr)
		}

		return true, err
	}

	return true, nil
}

func (p *WebhookProcessor) processEvent(ctx context.Context, event *webhook.Event) error {
	var payload xenditprovider.PaymentWebhook

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("decode webhook: %w", err)
	}

	return p.transactionManager.WithinTransaction(
		ctx,
		func(db database.DBTX) error {
			attemptRepository := paymentpostgres.NewAttemptRepository(db)
			paymentRepository := paymentpostgres.NewRepository(db)
			webhookRepository := webhookpostgres.NewRepository(db)

			attempt, err := attemptRepository.FindAttemptForProviderEvent(
				ctx,
				payment.ProviderXendit,
				payload.Data.PaymentID,
				payload.Data.PaymentRequestID,
				payload.Data.ReferenceID,
			)

			if err != nil {
				return err
			}

			intent, err := paymentRepository.GetByID(ctx, attempt.PaymentIntentID)

			if err != nil {
				return err
			}

			if err := applyXenditPaymentEvent(attempt, intent, payload); err != nil {
				return err
			}

			if payload.Data.PaymentID != "" {
				paymentID := payload.Data.PaymentID
				attempt.ProviderPaymentID = &paymentID
			}

			if payload.Data.PaymentRequestID != "" {
				paymentRequestID := payload.Data.PaymentRequestID
				attempt.ProviderPaymentRequestID = &paymentRequestID
			}

			expectedAttemptVersion := attempt.Version

			if err := attemptRepository.UpdateAttempt(ctx, attempt, expectedAttemptVersion); err != nil {
				return err
			}

			if err := paymentRepository.Update(
				ctx,
				intent,
				expectedAttemptVersion,
			); err != nil {
				return err
			}

			if err := webhookRepository.MarkProcessed(
				ctx,
				event.ID,
			); err != nil {
				return err
			}

			return nil
		},
	)
}

func applyXenditPaymentEvent(attempt *payment.PaymentAttempt, intent *payment.PaymentIntent, event xenditprovider.PaymentWebhook) error {
	switch event.Event {
	case "payment.authorization":
		return applyAuthorization(attempt, intent)

	case "payment.capture":
		return applyCapture(attempt, intent, event)

	case "payment.failure":
		return applyFailure(attempt, intent, event)

	default:
		return fmt.Errorf("unsupported payment event: %s", event.Event)
	}
}

func applyAuthorization(attempt *payment.PaymentAttempt, intent *payment.PaymentIntent) error {
	if attempt.Status == payment.AttemptStatusCaptured {
		return nil
	}

	if intent.Status == payment.StatusCaptured {
		return nil
	}

	if attempt.Status != payment.AttemptStatusAuthorized {
		if err := attempt.TransitionTo(payment.AttemptStatusAuthorized); err != nil {
			return err
		}
	}

	if intent.Status != payment.StatusAuthorized {
		if err := intent.TransitionTo(payment.StatusAuthorized); err != nil {
			return err
		}
	}

	return nil
}

func applyCapture(attempt *payment.PaymentAttempt, intent *payment.PaymentIntent, event xenditprovider.PaymentWebhook) error {
	if attempt.Status != payment.AttemptStatusCaptured {
		if err := attempt.TransitionTo(payment.AttemptStatusCaptured); err != nil {
			return err
		}
	}

	if intent.Status != payment.StatusCaptured {
		if err := intent.TransitionTo(payment.StatusCaptured); err != nil {
			return err
		}
	}

	capturedAMount := int64(0)

	for _, capture := range event.Data.Captures {
		capturedAMount += capture.CaptureAmount
	}

	if capturedAMount <= 0 {
		capturedAMount = event.Data.RequestAmount
	}

	if capturedAMount > intent.Amount {
		return fmt.Errorf("captured amount %d exceeds payment amount %d", capturedAMount, intent.Amount)
	}

	intent.CapturedAmount = capturedAMount

	return nil
}

func applyFailure(attempt *payment.PaymentAttempt, intent *payment.PaymentIntent, event xenditprovider.PaymentWebhook) error {
	if attempt.Status == payment.AttemptStatusCaptured {
		return nil
	}

	if intent.Status == payment.StatusCaptured {
		return nil
	}

	if event.Data.FailureCode != "" {
		code := event.Data.FailureCode
		attempt.ErrorCode = &code
	}

	if attempt.Status != payment.AttemptStatusFailed {
		if err := attempt.TransitionTo(payment.AttemptStatusFailed); err != nil {
			return err
		}
	}

	if intent.Status != payment.StatusFailed {
		if err := intent.TransitionTo(payment.StatusFailed); err != nil {
			return err
		}
	}

	return nil
}
