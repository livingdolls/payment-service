package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/livingdolls/payment-service/internal/database"
	"github.com/livingdolls/payment-service/internal/modules/payment"
	paymentpostgres "github.com/livingdolls/payment-service/internal/modules/payment/postgres"
	"github.com/livingdolls/payment-service/internal/provider"
)

type ProcessPaymentCommand struct {
	AttemptID         string
	Country           string
	ChannelCode       string
	ChannelProperties map[string]any
	Description       string
	Metadata          map[string]any
}

type ProcessPaymentResult struct {
	Payment *payment.PaymentIntent
	Attempt *payment.PaymentAttempt

	Actions []provider.Action
}

type ProcessPaymentUseCase struct {
	transactionManager database.TransactionManager
	paymentProvider    provider.PaymentProvider
}

func NewProcessPaymentUseCase(transactionManager database.TransactionManager, paymentProvider provider.PaymentProvider) *ProcessPaymentUseCase {
	return &ProcessPaymentUseCase{
		transactionManager: transactionManager,
		paymentProvider:    paymentProvider,
	}
}

func (u *ProcessPaymentUseCase) Execute(ctx context.Context, command ProcessPaymentCommand) (*ProcessPaymentResult, error) {
	channelCode := strings.TrimSpace(command.ChannelCode)

	if channelCode == "" {
		return nil, ErrChannelCodeRequired
	}

	country := strings.ToUpper(strings.TrimSpace(command.Country))

	if country == "" {
		country = "ID"
	}

	var providerInput provider.CreatePaymentInput

	err := u.transactionManager.WithinTransaction(
		ctx,
		func(db database.DBTX) error {
			attemptRepository := paymentpostgres.NewAttemptRepository(db)
			paymentRepository := paymentpostgres.NewRepository(db)

			attempt, err := attemptRepository.GetAttemptByID(ctx, command.AttemptID)

			if err != nil {
				return fmt.Errorf("get payment attempt: %w", err)
			}

			if attempt.Status != payment.AttemptStatusCreated {
				return fmt.Errorf("%w: current status %s", ErrPaymentAttemptNotProcessable, attempt.Status)
			}

			intent, err := paymentRepository.GetByID(ctx, attempt.PaymentIntentID)

			if err != nil {
				return fmt.Errorf("get payment intent: %w", err)
			}

			providerInput = provider.CreatePaymentInput{
				ReferenceID:       intent.ReferenceID,
				Amount:            intent.Amount,
				Currency:          intent.Currency,
				CaptureMethod:     string(intent.CaptureMethod),
				Country:           country,
				ChannelCode:       channelCode,
				ChannelProperties: command.ChannelProperties,
				Description:       command.Description,
				Metadata:          command.Metadata,
			}

			providerRequest, err := json.Marshal(providerInput)

			if err != nil {
				return fmt.Errorf("marshal provider request: %w", err)
			}

			attempt.ProviderRequest = providerRequest

			expectedAttemptVersion := attempt.Version

			if err := attempt.TransitionTo(payment.AttemptStatusRequestingProvider); err != nil {
				return err
			}

			if err := attemptRepository.UpdateAttempt(ctx, attempt, expectedAttemptVersion); err != nil {
				return err
			}

			expectedPaymentVersion := intent.Version

			if err := intent.TransitionTo(payment.StatusProcessing); err != nil {
				return err
			}

			if err := paymentRepository.Update(ctx, intent, expectedPaymentVersion); err != nil {
				return err
			}

			return nil
		},
	)

	if err != nil {
		return nil, err
	}

	providerResult, providerErr := u.paymentProvider.CreatePayment(
		ctx,
		providerInput,
	)

	if providerErr != nil {
		if err := u.applyProviderError(ctx, command.AttemptID, providerErr); err != nil {
			return nil, fmt.Errorf("apply provider error: %w", err)
		}

		return nil, providerErr
	}

	return u.applyProviderResult(ctx, command.AttemptID, providerResult)
}

func (u *ProcessPaymentUseCase) applyProviderError(ctx context.Context, attemptID string, err error) error {
	var providerErr *provider.Error

	if !errors.As(err, &providerErr) {
		return fmt.Errorf("unexpected provider error: %w", err)
	}

	return u.transactionManager.WithinTransaction(
		ctx,
		func(db database.DBTX) error {
			attemptRepository := paymentpostgres.NewAttemptRepository(db)
			paymentRepository := paymentpostgres.NewRepository(db)

			attempt, err := attemptRepository.GetAttemptByID(
				ctx,
				attemptID,
			)

			if err != nil {
				return err
			}

			intent, err := paymentRepository.GetByID(ctx, attempt.PaymentIntentID)

			if err != nil {
				return err
			}

			expectedAttemptVersion := attempt.Version

			if providerErr.Code != "" {
				code := providerErr.Code
				attempt.ErrorCode = &code
			}

			if providerErr.Message != "" {
				message := providerErr.Message
				attempt.ErrorMessage = &message
			}

			switch providerErr.Kind {
			case provider.ErrorKindRejected:
				if err := attempt.TransitionTo(payment.AttemptStatusFailed); err != nil {
					return err
				}

			case provider.ErrorKindUnknownOutcome:
				if err := attempt.TransitionTo(payment.AttemptStatusUnknown); err != nil {
					return err
				}

			default:
				return fmt.Errorf(
					"unknown provider error kind: %s", providerErr.Kind,
				)
			}

			if err := attemptRepository.UpdateAttempt(ctx, attempt, expectedAttemptVersion); err != nil {
				return err
			}

			if providerErr.Kind == provider.ErrorKindRejected {
				expectedPaymentVersion := intent.Version

				if err := intent.TransitionTo(payment.StatusFailed); err != nil {
					return err
				}

				if err := paymentRepository.Update(
					ctx,
					intent,
					expectedPaymentVersion,
				); err != nil {
					return err
				}
			}

			return nil
		},
	)
}

func (u *ProcessPaymentUseCase) applyProviderResult(ctx context.Context, attemptID string, result *provider.CreatePaymentResult) (*ProcessPaymentResult, error) {
	var output *ProcessPaymentResult

	err := u.transactionManager.WithinTransaction(
		ctx,
		func(db database.DBTX) error {
			attemptRepository := paymentpostgres.NewAttemptRepository(db)
			paymentRepository := paymentpostgres.NewRepository(db)

			attempt, err := attemptRepository.GetAttemptByID(ctx, attemptID)

			if err != nil {
				return err
			}

			intent, err := paymentRepository.GetByID(ctx, attempt.PaymentIntentID)

			if err != nil {
				return err
			}

			paymentRequestID := result.PaymentRequestID
			attempt.ProviderPaymentRequestID = &paymentRequestID
			attempt.ProviderPaymentID = result.PaymentID
			attempt.ProviderResponse = result.RawResponse

			nextAttemptStatus, nextPaymentStatus := mapProviderResultStatus(result.Status)
			expectedAttemptVersion := attempt.Version

			if err := attempt.TransitionTo(nextAttemptStatus); err != nil {
				return err
			}

			if err := attemptRepository.UpdateAttempt(
				ctx,
				attempt,
				expectedAttemptVersion,
			); err != nil {
				return err
			}

			if nextPaymentStatus != "" && intent.Status != nextPaymentStatus {
				expectedPaymentVersion := intent.Version

				if err := intent.TransitionTo(nextPaymentStatus); err != nil {
					return err
				}

				if nextPaymentStatus == payment.StatusCaptured {
					intent.CapturedAmount = intent.Amount
				}

				if err := paymentRepository.Update(
					ctx,
					intent,
					expectedPaymentVersion,
				); err != nil {
					return err
				}

			}

			output = &ProcessPaymentResult{
				Payment: intent,
				Attempt: attempt,
				Actions: result.Actions,
			}

			return nil
		},
	)

	if err != nil {
		return nil, err
	}
	return output, nil
}

func mapProviderResultStatus(status provider.PaymentStatus) (payment.AttemptStatus, payment.Status) {
	switch status {
	case provider.PaymentStatusPending:
		return payment.AttemptStatusPending, payment.StatusProcessing

	case provider.PaymentStatusRequiresAction:
		return payment.AttemptStatusRequiresAction, payment.StatusRequiresAction

	case provider.PaymentStatusAuthorized:
		return payment.AttemptStatusAuthorized, payment.StatusAuthorized

	case provider.PaymentStatusSucceeded:
		return payment.AttemptStatusCaptured, payment.StatusCaptured

	case provider.PaymentStatusFail:
		return payment.AttemptStatusFailed, payment.StatusFailed

	case provider.PaymentStatusExpired:
		return payment.AttemptStatusExpired, payment.StatusExpired

	case provider.PaymentStatusCanceled:
		return payment.AttemptStatusCanceled, payment.StatusCanceled

	default:
		return payment.AttemptStatusUnknown, payment.StatusProcessing
	}
}
