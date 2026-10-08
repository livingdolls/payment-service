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

const providerStateApplyMaxRetries = 3

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

type PaymentProcessor interface {
	Execute(ctx context.Context, command ProcessPaymentCommand) (*ProcessPaymentResult, error)
}

var _ PaymentProcessor = (*ProcessPaymentUseCase)(nil)

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
		result, resolvedByNewerState, err := u.applyProviderError(ctx, command.AttemptID, providerErr)

		if err != nil {
			return nil, fmt.Errorf("apply provider error: %w", err)
		}

		if resolvedByNewerState {
			return result, nil
		}

		return nil, providerErr
	}

	return u.applyProviderResult(ctx, command.AttemptID, providerResult)
}

func (u *ProcessPaymentUseCase) applyProviderError(ctx context.Context, attemptID string, err error) (*ProcessPaymentResult, bool, error) {
	var providerErr *provider.Error

	if !errors.As(err, &providerErr) {
		return nil, false, fmt.Errorf("unexpected provider error: %w", err)
	}

	var lastErr error

	for attemptNumber := 1; attemptNumber <= providerStateApplyMaxRetries; attemptNumber++ {
		result, resolved, applyErr := u.applyProviderErrorOnce(ctx, attemptID, providerErr)

		if applyErr == nil {
			return result, resolved, nil
		}

		if !errors.Is(applyErr, payment.ErrAttemptConcurrentUpdate) && !errors.Is(applyErr, payment.ErrPaymentConcurrentUpdate) {
			return nil, false, applyErr
		}

		lastErr = applyErr
	}

	return nil, false, fmt.Errorf(
		"apply provider error after %d retries %w",
		providerStateApplyMaxRetries,
		lastErr,
	)
}

func (u *ProcessPaymentUseCase) applyProviderErrorOnce(ctx context.Context, attemptID string, providerErr *provider.Error) (*ProcessPaymentResult, bool, error) {
	var output *ProcessPaymentResult
	var resolvedByNewerState bool

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

			/*
				Jika webhook sudah membawa state
				lebih maju, provider error lama
				tidak lagi authoritative.
			*/

			switch attempt.Status {
			case payment.AttemptStatusPending,
				payment.AttemptStatusRequiresAction,
				payment.AttemptStatusAuthorized,
				payment.AttemptStatusCaptured,
				payment.AttemptStatusFailed,
				payment.AttemptStatusExpired,
				payment.AttemptStatusCanceled:

				resolvedByNewerState = true

				output = &ProcessPaymentResult{
					Payment: intent,
					Attempt: attempt,
				}

				return nil
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

			if len(providerErr.RawResponse) > 0 {
				attempt.ProviderResponse = providerErr.RawResponse
			}

			switch providerErr.Kind {
			case provider.ErrorKindRejected:
				if attempt.Status != payment.AttemptStatusFailed {
					if err := attempt.TransitionTo(payment.AttemptStatusFailed); err != nil {
						return err
					}
				}

			case provider.ErrorKindUnknownOutcome:
				if attempt.Status != payment.AttemptStatusUnknown {
					if err := attempt.TransitionTo(payment.AttemptStatusUnknown); err != nil {
						return err
					}
				}

			default:
				return fmt.Errorf("unknown provider error kind: %s", providerErr.Kind)
			}

			if err := attemptRepository.UpdateAttempt(ctx, attempt, expectedAttemptVersion); err != nil {
				return err
			}

			/*
				UNKNOWN tidak membuat intent FAILED.

				Intent tetap PROCESSING.
			*/

			if providerErr.Kind == provider.ErrorKindRejected {
				expectedPaymentVersion := intent.Version

				if intent.Status != payment.StatusFailed {
					if err := intent.TransitionTo(payment.StatusFailed); err != nil {
						return err
					}

					if err := paymentRepository.Update(ctx, intent, expectedPaymentVersion); err != nil {
						return err
					}
				}
			}

			output = &ProcessPaymentResult{
				Payment: intent,
				Attempt: attempt,
			}

			return nil
		},
	)

	if err != nil {
		return nil, false, err
	}

	return output, resolvedByNewerState, nil
}

func (u *ProcessPaymentUseCase) applyProviderResult(ctx context.Context, attemptID string, result *provider.CreatePaymentResult) (*ProcessPaymentResult, error) {
	var lastErr error

	for attemptNumber := 1; attemptNumber <= providerStateApplyMaxRetries; attemptNumber++ {
		output, err := u.applyProviderResultOnce(
			ctx,
			attemptID,
			result,
		)

		if err == nil {
			return output, nil
		}

		if !errors.Is(
			err,
			payment.ErrAttemptConcurrentUpdate,
		) && !errors.Is(err, payment.ErrPaymentConcurrentUpdate) {
			return nil, err
		}

		lastErr = err
	}

	return nil, fmt.Errorf("apply provider result after %d retries: %w", providerStateApplyMaxRetries, lastErr)
}

func (u *ProcessPaymentUseCase) applyProviderResultOnce(ctx context.Context, attemptID string, result *provider.CreatePaymentResult) (*ProcessPaymentResult, error) {
	var output *ProcessPaymentResult

	err := u.transactionManager.WithinTransaction(
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

			intent, err := paymentRepository.GetByID(
				ctx,
				attempt.PaymentIntentID,
			)
			if err != nil {
				return err
			}

			if err := bindProviderResultIdentifiers(attempt, result); err != nil {
				return err
			}

			/*
				Raw response tetap boleh disimpan,
				walaupun state response stale.
			*/

			if len(result.RawResponse) > 0 {
				attempt.ProviderResponse = result.RawResponse
			}

			nextAttemptStatus, nextPaymentStatus := mapProviderResultStatus(result.Status)

			attemptStale, err := isStaleAttemptState(attempt.Status, nextAttemptStatus)

			if err != nil {
				return err
			}

			paymentStale, err := isStalePaymentState(intent.Status, nextPaymentStatus)

			if err != nil {
				return err
			}

			/*
				Jika salah satu sisi sudah lebih maju,
				anggap response provider stale.

				Jangan turunkan state.
			*/

			stale := attemptStale || paymentStale

			expectedAttemptVersion := attempt.Version

			if !stale && attempt.Status != nextAttemptStatus {
				if err := attempt.TransitionTo(nextAttemptStatus); err != nil {
					return err
				}
			}

			/*
				Tetap update attempt untuk menyimpan
				provider IDs dan raw response.
			*/

			if err := attemptRepository.UpdateAttempt(
				ctx,
				attempt,
				expectedAttemptVersion,
			); err != nil {
				return err
			}

			if !stale && intent.Status != nextPaymentStatus {
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

			var actions []provider.Action

			if !stale {
				actions = result.Actions
			}

			output = &ProcessPaymentResult{
				Payment: intent,
				Attempt: attempt,
				Actions: actions,
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

func attemptProgressRank(status payment.AttemptStatus) (int, bool) {
	switch status {
	case payment.AttemptStatusCreated:
		return 0, true
	case payment.AttemptStatusRequestingProvider:
		return 10, true
	case payment.AttemptStatusPending:
		return 20, true
	case payment.AttemptStatusRequiresAction:
		return 30, true
	case payment.AttemptStatusAuthorized:
		return 40, true
	case payment.AttemptStatusCaptured:
		return 50, true
	default:
		return 0, false
	}
}

func paymentProgressRank(status payment.Status) (int, bool) {
	switch status {
	case payment.StatusCreated:
		return 0, true
	case payment.StatusProcessing:
		return 10, true
	case payment.StatusRequiresAction:
		return 20, true
	case payment.StatusAuthorized:
		return 30, true
	case payment.StatusCaptured:
		return 40, true
	default:
		return 0, false
	}
}

func isStaleAttemptState(current payment.AttemptStatus, incoming payment.AttemptStatus) (bool, error) {
	if current == incoming {
		return false, nil
	}

	if current == payment.AttemptStatusCaptured {
		return true, nil
	}

	switch current {
	case payment.AttemptStatusFailed, payment.AttemptStatusExpired, payment.AttemptStatusCanceled:

		if incoming == payment.AttemptStatusCaptured {
			return false, fmt.Errorf("%w: attempt current=%s incoming=%s", ErrProviderStateConflict, current, incoming)
		}

		return true, nil
	}

	if current == payment.AttemptStatusUnknown {
		return false, nil
	}

	if incoming == payment.AttemptStatusUnknown {
		if current != payment.AttemptStatusRequestingProvider {
			return true, nil
		}

		return false, nil
	}

	currentRank, currentOK := attemptProgressRank(current)
	incomingRank, incomingOK := attemptProgressRank(incoming)

	if currentOK && incomingOK && currentRank > incomingRank {
		return true, nil
	}

	return false, nil
}

func isStalePaymentState(current payment.Status, incoming payment.Status) (bool, error) {
	if current == incoming {
		return false, nil
	}

	if current == payment.StatusCaptured {
		return true, nil
	}

	switch current {
	case payment.StatusFailed, payment.StatusExpired, payment.StatusCanceled:
		if incoming == payment.StatusCaptured {
			return false, fmt.Errorf("%w: payment current=%s incoming=%s", ErrProviderStateConflict, current, incoming)
		}

		return true, nil
	}

	currentRank, currentOK := paymentProgressRank(current)
	incomingRank, incomingOK := paymentProgressRank(incoming)

	if currentOK && incomingOK && currentRank > incomingRank {
		return true, nil
	}

	return false, nil
}

func bindProviderResultIdentifiers(attempt *payment.PaymentAttempt, result *provider.CreatePaymentResult) error {
	if result.PaymentRequestID != "" {
		if attempt.ProviderPaymentRequestID != nil && *attempt.ProviderPaymentRequestID != result.PaymentRequestID {
			return fmt.Errorf("%w: existing payment_request_id=%s incoming=%s", ErrProviderIdentityMismatch, *attempt.ProviderPaymentRequestID, result.PaymentRequestID)
		}

		paymentRequestID := result.PaymentRequestID
		attempt.ProviderPaymentRequestID = &paymentRequestID
	}

	if result.PaymentID != nil && *result.PaymentID != "" {
		if attempt.ProviderPaymentID != nil && *attempt.ProviderPaymentID != *result.PaymentID {
			return fmt.Errorf("%w: existing payment_id=%s incoming=%s", ErrProviderIdentityMismatch, *attempt.ProviderPaymentID, *result.PaymentID)
		}

		paymentID := *result.PaymentID
		attempt.ProviderPaymentID = &paymentID
	}

	return nil
}
