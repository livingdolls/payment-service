package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/livingdolls/payment-service/internal/modules/webhook"
)

type WebhookHandler struct {
	service *webhook.Service
}

func NewWebhookHandler(service *webhook.Service) *WebhookHandler {
	return &WebhookHandler{
		service: service,
	}
}

func (h *WebhookHandler) XenditPayment(c *gin.Context) {
	const maxWebhookSize = 1 << 20

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxWebhookSize)

	body, err := io.ReadAll(c.Request.Body)

	if err != nil {
		respondError(
			c,
			http.StatusBadRequest,
			"INVALID_WEBHOOK_BODY",
			"invalid webhook body",
		)

		return
	}

	result, err := h.service.ReceiveXenditPayment(
		c.Request.Context(),
		webhook.ReceiveInput{
			Token:   c.GetHeader("x-callback-token"),
			Payload: body,
		},
	)

	if err != nil {
		h.handleWebhookError(c, err)
		return
	}

	c.JSON(
		http.StatusOK,
		gin.H{
			"received":  true,
			"duplicate": result.Duplicate,
		},
	)
}

func (h *WebhookHandler) handleWebhookError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, webhook.ErrInvalidToken):
		respondError(
			c,
			http.StatusUnauthorized,
			"INVALID_WEBHOOK_TOKEN",
			"invalid webhook token",
		)

	case errors.Is(err, webhook.ErrInvalidPayload):
		respondError(
			c,
			http.StatusBadRequest,
			"INVALID_WEBHOOK_PAYLOAD",
			"invalid webhook payload",
		)

	case errors.Is(err, webhook.ErrUnsupportedEvent):
		c.JSON(
			http.StatusOK,
			gin.H{
				"received": true,
				"ignore":   true,
			},
		)

	default:
		respondError(
			c,
			http.StatusInternalServerError,
			"WEBHOOK_STORE_FAILED",
			"failed to store webhook",
		)
	}
}
