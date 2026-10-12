package http

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/livingdolls/payment-service/internal/modules/order"
	"github.com/livingdolls/payment-service/internal/modules/outbox"
)

type Handler struct {
	consumer *order.Consumer
	token    string
}

func NewHandler(consumer *order.Consumer, token string) *Handler {
	return &Handler{
		consumer: consumer,
		token:    token,
	}
}

func (h *Handler) ConsumePaymentEvent(c *gin.Context) {
	expected := "Bearer " + h.token

	if h.token == "" || subtle.ConstantTimeCompare([]byte(c.GetHeader("Authorization")), []byte(expected)) != 1 {
		c.JSON(
			http.StatusUnauthorized,
			gin.H{
				"error": "unauthorized",
			},
		)

		return
	}

	const maxBodySize = 1 << 20

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodySize)

	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()

	var event outbox.PaymentCapturedPayload

	if err := decoder.Decode(&event); err != nil {
		c.JSON(
			http.StatusBadRequest,
			gin.H{
				"error": "invalid event payment",
			},
		)

		return
	}

	// tolak JSON tambahan setelah object utama
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		c.JSON(
			http.StatusBadRequest,
			gin.H{
				"error": "unexpected trailing payload",
			},
		)

		return
	}

	// header harus cocok dengan payload
	if c.GetHeader("Idempotency-Key") != event.EventID ||
		c.GetHeader("X-Event-ID") != event.EventID ||
		c.GetHeader("X-Event-Type") != event.EventType {
		c.JSON(
			http.StatusBadRequest,
			gin.H{
				"error": "event headers missmatch",
			},
		)

		return
	}

	duplicate, err := h.consumer.Consume(
		c.Request.Context(),
		event,
	)

	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(
		http.StatusOK,
		gin.H{
			"received":  true,
			"duplicate": duplicate,
			"order_id":  event.OrderID,
		},
	)
}

func (h *Handler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, order.ErrInvalidEvent):
		c.JSON(
			http.StatusBadRequest,
			gin.H{"error": "invalid payment event"},
		)

	case errors.Is(err, order.ErrOrderNotFound):
		c.JSON(
			http.StatusNotFound,
			gin.H{"error": "order not found"},
		)

	case errors.Is(err, order.ErrPaymentNotBound):
		c.JSON(
			http.StatusConflict,
			gin.H{"error": "order payment not bound"},
		)

	case errors.Is(err, order.ErrPaymentMismatch):
		c.JSON(
			http.StatusConflict,
			gin.H{"error": "payment mismatch"},
		)

	case errors.Is(err, order.ErrOrderNotPayable):
		c.JSON(
			http.StatusConflict,
			gin.H{"error": "order cannot be paid"},
		)

	case errors.Is(err, order.ErrEventPayloadConflict):
		c.JSON(
			http.StatusConflict,
			gin.H{"error": "event payload conflict"},
		)

	default:
		c.JSON(
			http.StatusInternalServerError,
			gin.H{"error": "internal server error"},
		)
	}
}
