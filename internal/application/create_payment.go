package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/livingdolls/payment-service/internal/database"
	"github.com/livingdolls/payment-service/internal/modules/idempotency"
	idempotencypostgres "github.com/livingdolls/payment-service/internal/modules/idempotency/postgres"
	"github.com/livingdolls/payment-service/internal/modules/payment"
	paymentpostgres "github.com/livingdolls/payment-service/internal/modules/payment/postgres"
)

const (
	createPaymentOperation = "CREATE_PAYMENT_V1"
	idempotencyTTL         = 24 * time.Hour
)

type CreatePaymentCommand struct {
	IdempotencyKey string

	OrderID  string
	Amount   int64
	Currency string
}

type CreatePaymentResult struct {
	StatusCode int
	Body       []byte

	Replayed bool
}

type CreatePaymentUseCase struct {
	transactionManager database.TransactionManager
}

type createPaymentResponse struct {
	ID string `json:"id"`

	ReferenceID string `json:"reference_id"`
	OrderID     string `json:"order_id"`

	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`

	Status        payment.Status        `json:"status"`
	CaptureMethod payment.CaptureMethod `json:"capture_method"`

	CapturedAMount int64 `json:"captured_amount"`
	RefundedAmount int64 `json:"refunded_amount"`

	Version int64 `json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewCreatePaymentUseCase(transactionManager database.TransactionManager) *CreatePaymentUseCase {
	return &CreatePaymentUseCase{
		transactionManager: transactionManager,
	}
}

func (u *CreatePaymentUseCase) Execute(ctx context.Context, command CreatePaymentCommand) (*CreatePaymentResult, error) {
	key := strings.TrimSpace(command.IdempotencyKey)

	if key == "" {
		return nil, idempotency.ErrKeyRequired
	}

	if len(key) > 255 {
		return nil, idempotency.ErrKeyTooLong
	}

	requestHash, err := createPaymentRequestHash(command)

	if err != nil {
		return nil, err
	}

	var result *CreatePaymentResult

	err = u.transactionManager.WithinTransaction(
		ctx,
		func(db database.DBTX) error {
			idempotencyRepository := idempotencypostgres.NewRepository(db)

			record, created, err := idempotencyRepository.Acquire(
				ctx,
				createPaymentOperation,
				key,
				requestHash,
				time.Now().UTC().Add(idempotencyTTL),
			)

			if err != nil {
				return fmt.Errorf(
					"acquire idempotency key: %w", err,
				)
			}

			if !created {
				if record.RequestHash != requestHash {
					return idempotency.ErrConflict
				}

				switch record.Status {
				case idempotency.StatusCompleted:
					result = &CreatePaymentResult{
						StatusCode: dereferenceStatus(record.ResponseStatus),
						Body:       record.ResponseBody,
						Replayed:   true,
					}

					return nil

				case idempotency.StatusProcessing:
					return idempotency.ErrInProgress

				default:
					return fmt.Errorf("unknown idempotency status: %s", record.Status)
				}
			}

			paymentRepository := paymentpostgres.NewRepository(db)
			paymentService := payment.NewService(paymentRepository)

			paymentIntent, err := paymentService.Create(
				ctx,
				payment.CreateInput{
					OrderID:  command.OrderID,
					Amount:   command.Amount,
					Currency: command.Currency,
				},
			)

			if err != nil {
				return err
			}

			response := newCreatePaymentResponse(paymentIntent)

			body, err := json.Marshal(response)
			if err != nil {
				return fmt.Errorf("marshal payment response: %w", err)
			}

			const statusCode = http.StatusCreated

			if err := idempotencyRepository.Complete(
				ctx,
				record.ID,
				statusCode,
				body,
			); err != nil {
				return fmt.Errorf("complete idempotency: %w", err)
			}

			result = &CreatePaymentResult{
				StatusCode: statusCode,
				Body:       body,
				Replayed:   false,
			}

			return nil
		},
	)

	if err != nil {
		return nil, err
	}

	return result, nil
}

func newCreatePaymentResponse(p *payment.PaymentIntent) createPaymentResponse {
	return createPaymentResponse{
		ID: p.ID,

		ReferenceID: p.ReferenceID,
		OrderID:     p.OrderID,

		Amount:   p.Amount,
		Currency: p.Currency,

		Status:        p.Status,
		CaptureMethod: p.CaptureMethod,

		CapturedAMount: p.CapturedAmount,
		RefundedAmount: p.RefundedAmount,

		Version: p.Version,

		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}

func createPaymentRequestHash(command CreatePaymentCommand) (string, error) {
	payload := struct {
		OrderID  string `json:"order_id"`
		Amount   int64  `json:"amount"`
		Currency string `json:"currency"`
	}{
		OrderID:  strings.TrimSpace(command.OrderID),
		Amount:   command.Amount,
		Currency: strings.ToUpper(strings.TrimSpace(command.Currency)),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal idempotency payload: %w", err)
	}

	hash := sha256.Sum256(data)

	return hex.EncodeToString(hash[:]), nil
}

func dereferenceStatus(status *int) int {
	if status == nil {
		return http.StatusInternalServerError
	}

	return *status
}
