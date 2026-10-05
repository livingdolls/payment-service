package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/livingdolls/payment-service/internal/modules/payment"
)

type PaymentHandler struct {
	service payment.Creator
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

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func NewPaymentHandler(service payment.Creator) *PaymentHandler {
	return &PaymentHandler{
		service: service,
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

	input := payment.CreateInput{
		OrderID:  request.OrderID,
		Amount:   request.Amount,
		Currency: request.Currency,
	}

	result, err := h.service.Create(c.Request.Context(), input)

	if err != nil {
		h.handleCreateError(c, err)
		return
	}

	c.JSON(
		http.StatusCreated,
		newPaymentResponse(result),
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
	default:
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}
