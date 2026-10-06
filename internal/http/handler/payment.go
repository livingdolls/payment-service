package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/livingdolls/payment-service/internal/application"
	"github.com/livingdolls/payment-service/internal/modules/idempotency"
	"github.com/livingdolls/payment-service/internal/modules/payment"
	"github.com/livingdolls/payment-service/internal/provider"
)

type PaymentHandler struct {
	createPaymentUseCase  *application.CreatePaymentUseCase
	processPaymentUseCase application.PaymentProcessor
}

type createPaymentRequest struct {
	OrderID  string `json:"order_id"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

type paymentResponse struct {
	ID          string `json:"id"`
	ReferenceID string `json:"reference_id"`
	OrderID     string `json:"order_id"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`

	Status         payment.Status        `json:"status"`
	CaptureMethod  payment.CaptureMethod `json:"capture_method"`
	CapturedAmount int64                 `json:"captured_amount"`
	RefundedAmount int64                 `json:"refunded_amount"`

	Version int64 `json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type processPaymentRequest struct {
	Country           string         `json:"country"`
	ChannelCode       string         `json:"channel_code"`
	ChannelProperties map[string]any `json:"channel_properties"`
	Description       string         `json:"description"`
	Metadata          map[string]any `json:"metadata"`
}

type processPaymentResponse struct {
	Payment struct {
		ID          string         `json:"id"`
		ReferenceID string         `json:"reference_id"`
		Status      payment.Status `json:"status"`
	} `json:"payment"`

	Attempt struct {
		ID                       string                `json:"id"`
		Status                   payment.AttemptStatus `json:"status"`
		Provider                 payment.Provider      `json:"provider"`
		ProviderPaymentRequestID *string               `json:"provider_payment_request_id,omitempty"`
		ProviderPaymentID        *string               `json:"provider_payment_id,omitempty"`
	} `json:"attempt"`

	Actions []provider.Action `json:"actions"`
}

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func NewPaymentHandler(createPaymentUsecase *application.CreatePaymentUseCase, processPaymentUsecase application.PaymentProcessor) *PaymentHandler {
	return &PaymentHandler{
		createPaymentUseCase:  createPaymentUsecase,
		processPaymentUseCase: processPaymentUsecase,
	}
}

func (h *PaymentHandler) Create(c *gin.Context) {
	var request createPaymentRequest

	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(
			c,
			http.StatusBadRequest,
			"INVALID_REQUEST",
			"invalid request body",
		)

		return
	}

	result, err := h.createPaymentUseCase.Execute(
		c.Request.Context(),
		application.CreatePaymentCommand{
			IdempotencyKey: c.GetHeader("Idempotency-Key"),
			OrderID:        request.OrderID,
			Amount:         request.Amount,
			Currency:       request.Currency,
		},
	)

	if err != nil {
		h.handleCreateError(c, err)
		return
	}

	c.Data(
		result.StatusCode,
		"application/json; charset=utf-8",
		result.Body,
	)
}

func (h *PaymentHandler) Process(c *gin.Context) {
	attemptID := strings.TrimSpace(
		c.Param("attempt_id"),
	)

	if attemptID == "" {
		respondError(
			c,
			http.StatusBadRequest,
			"ATTEMPT_ID_REQUIRED",
			"attempt id is required",
		)

		return
	}

	var request processPaymentRequest

	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(
			c,
			http.StatusBadRequest,
			"INVALID_REQUEST",
			"invalid request body",
		)

		return
	}

	result, err := h.processPaymentUseCase.Execute(
		c.Request.Context(),
		application.ProcessPaymentCommand{
			AttemptID:         attemptID,
			Country:           request.Country,
			ChannelCode:       request.ChannelCode,
			ChannelProperties: request.ChannelProperties,
			Description:       request.Description,
			Metadata:          request.Metadata,
		},
	)

	if err != nil {
		h.handleProcessError(
			c,
			err,
			attemptID,
		)
		return
	}

	c.JSON(
		http.StatusOK,
		newProcessPaymentResponse(
			result,
		),
	)
}

func newPaymentResponse(p *payment.PaymentIntent) paymentResponse {
	return paymentResponse{
		ID:          p.ID,
		ReferenceID: p.ReferenceID,
		OrderID:     p.OrderID,
		Amount:      p.Amount,
		Currency:    p.Currency,

		Status:         p.Status,
		CaptureMethod:  p.CaptureMethod,
		CapturedAmount: p.CapturedAmount,
		RefundedAmount: p.RefundedAmount,

		Version:   p.Version,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}

func newProcessPaymentResponse(result *application.ProcessPaymentResult) processPaymentResponse {
	var response processPaymentResponse

	response.Payment.ID = result.Payment.ID
	response.Payment.ReferenceID = result.Payment.ReferenceID
	response.Payment.Status = result.Payment.Status
	response.Attempt.ID = result.Attempt.ID
	response.Attempt.Status = result.Attempt.Status
	response.Attempt.Provider = result.Attempt.Provider
	response.Attempt.ProviderPaymentRequestID = result.Attempt.ProviderPaymentRequestID
	response.Attempt.ProviderPaymentID = result.Attempt.ProviderPaymentID
	response.Actions = result.Actions

	return response
}

func respondError(c *gin.Context, status int, code string, message string) {
	var response errorResponse

	response.Error.Code = code
	response.Error.Message = message

	c.JSON(status, response)
}

func (h *PaymentHandler) handleCreateError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, payment.ErrOrderIDRequired):
		respondError(c, http.StatusUnprocessableEntity, "ORDER_ID_REQUIRED", "order_id is required")
	case errors.Is(err, payment.ErrOrderIDTooLong):
		respondError(c, http.StatusUnprocessableEntity, "ORDER_ID_TOO_LONG", "order_id is too long")
	case errors.Is(err, payment.ErrInvalidAmount):
		respondError(c, http.StatusUnprocessableEntity, "INVALID_AMOUNT", "amount must be greater than zero")
	case errors.Is(err, payment.ErrInvalidCurrency):
		respondError(c, http.StatusUnprocessableEntity, "INVALID_CURRENCY", "currency is invalid")
	case errors.Is(err, payment.ErrUnsupportedCurrency):
		respondError(c, http.StatusUnprocessableEntity, "UNSUPPORTED_CURRENCY", "currency is not supported")
	case errors.Is(err, idempotency.ErrKeyRequired):
		respondError(c, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required")
	case errors.Is(err, idempotency.ErrKeyTooLong):
		respondError(c, http.StatusBadRequest, "IDEMPOTENCY_KEY_TOO_LONG", "Idempotency-Key is too long")
	case errors.Is(err, idempotency.ErrConflict):
		respondError(c, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "idempotency key was already used with a different request")
	case errors.Is(err, idempotency.ErrInProgress):
		respondError(c, http.StatusConflict, "IDEMPOTENCY_IN_PROGRESS", "request with this idempotency key is still processing")
	default:
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

func (h *PaymentHandler) handleProcessError(c *gin.Context, err error, attemptID string) {
	switch {
	case errors.Is(err, application.ErrChannelCodeRequired):
		respondError(
			c,
			http.StatusUnprocessableEntity,
			"CHANNEL_CODE_REQUIRED",
			"channel_code is required",
		)

	case errors.Is(err, payment.ErrAttemptNotFound):
		respondError(
			c,
			http.StatusNotFound,
			"PAYMENT_ATTEMPT_NOT_FOUND",
			"payment attempt not found",
		)

	case errors.Is(err, application.ErrPaymentAttemptNotProcessable):
		respondError(
			c,
			http.StatusConflict,
			"PAYMENT_ATTEMPT_NOT_PROCESSABLE",
			"payment attempt cannot be process from its current status",
		)

	default:
		var providerErr *provider.Error
		if errors.As(err, &providerErr) {
			h.handleProviderError(c, providerErr, attemptID)

			return
		}

		respondError(
			c,
			http.StatusInternalServerError,
			"INTERNAL_ERROR",
			"internal server error",
		)
	}
}

func (h *PaymentHandler) handleProviderError(c *gin.Context, err *provider.Error, attemptID string) {
	switch err.Kind {
	case provider.ErrorKindRejected:
		respondError(
			c,
			http.StatusUnprocessableEntity,
			"PAYMENT_PROVIDER_REJECTED",
			"payment request was rejected by payment provider",
		)

	case provider.ErrorKindUnknownOutcome:
		c.JSON(
			http.StatusAccepted,
			gin.H{
				"attempt_id": attemptID,
				"status":     "PROCESSING",
				"code":       "PAYMENT_OUTCOME_UNKNOWN",
				"message":    "payment outcome is being reconciled",
			},
		)

	default:
		respondError(
			c,
			http.StatusBadGateway,
			"PAYMENT_PROVIDER_ERROR",
			"payment provider error",
		)
	}
}
