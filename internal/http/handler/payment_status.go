package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/livingdolls/payment-service/internal/provider"
)

type PaymentStatusHandler struct {
	db *pgxpool.Pool
}

func NewPaymentStatusHandler(db *pgxpool.Pool) *PaymentStatusHandler {
	return &PaymentStatusHandler{
		db: db,
	}
}

type internalPaymentStatus struct {
	PaymentIntentID string `json:"payment_intent_id"`
	AttemptID       string `json:"attempt_id"`

	ReferenceID string `json:"reference_id"`
	OrderID     string `json:"order_id"`

	Amount         int64  `json:"amount"`
	Currency       string `json:"currency"`
	CapturedAmount int64  `json:"captured_amount"`

	PaymentStatus string `json:"payment_status"`
	AttemptStatus string `json:"attempt_status"`

	Actions []provider.Action `json:"actions"`
}

func (h *PaymentStatusHandler) Get(c *gin.Context) {
	paymentID := c.Param("payment_id")

	if _, err := uuid.Parse(paymentID); err != nil {
		c.JSON(
			http.StatusBadRequest,
			gin.H{"error": "invalid payment ID"},
		)

		return
	}

	result, err := h.readStatus(c.Request.Context(), paymentID)

	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(
			http.StatusNotFound,
			gin.H{"error": "payment not found"},
		)

		return
	}

	if err != nil {
		c.JSON(
			http.StatusInternalServerError,
			gin.H{"error": "failed to read payment status"},
		)

		return
	}

	c.JSON(
		http.StatusOK,
		result,
	)
}

func (h *PaymentStatusHandler) readStatus(ctx context.Context, paymentID string) (*internalPaymentStatus, error) {
	const query = `
		SELECT
			p.id::text,
			a.id::text,
			p.reference_id,
			p.order_id,
			p.amount,
			p.currency,
			p.captured_amount,
			p.status,
			a.status,
			a.provider_response
		FROM payment_intents p

		JOIN LATERAL (
			SELECT
				id,
				status,
				provider_response
			FROM payment_attempts
			WHERE payment_intent_id = p.id
			ORDER BY attempt_number DESC
		)
	`

	result := &internalPaymentStatus{
		Actions: make([]provider.Action, 0),
	}

	var rawProviderResponse []byte

	err := h.db.QueryRow(
		ctx,
		query,
		paymentID,
	).Scan(
		&result.PaymentIntentID,
		&result.AttemptID,
		&result.ReferenceID,
		&result.OrderID,
		&result.Amount,
		&result.Currency,
		&result.CapturedAmount,
		&result.PaymentStatus,
		&result.AttemptStatus,
		&rawProviderResponse,
	)

	if err != nil {
		return nil, err
	}

	// Hanya berikan actions jika user memang
	// masih harus melakukan tindakan pembayaran.
	if result.AttemptStatus == "REQUIRES_ACTION" && len(rawProviderResponse) > 0 {
		var raw struct {
			Actions []provider.Action `json:"actions"`
		}

		if err := json.Unmarshal(
			rawProviderResponse,
			&raw,
		); err != nil {
			return nil, err
		}

		for _, action := range raw.Actions {
			switch action.Descriptor {
			case "QR_STRING", "WEB_URL":
				if action.Value != "" {
					result.Actions = append(result.Actions, action)
				}
			}
		}
	}

	return result, nil
}
