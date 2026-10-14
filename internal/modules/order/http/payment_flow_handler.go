package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/livingdolls/payment-service/internal/modules/order"
)

type PaymentFlowHandler struct {
	service *order.PaymentFlowService
}

func NewPaymentFlowHandler(service *order.PaymentFlowService) *PaymentFlowHandler {
	return &PaymentFlowHandler{
		service: service,
	}
}

type payRequest struct {
	ChannelCode string `json:"channel_code" binding:"required"`
}

func (h *PaymentFlowHandler) Status(c *gin.Context) {
	result, err := h.service.Status(c.Request.Context(), c.Param("order_id"), c.GetString("customer_id"))

	if err != nil {
		respondPaymentFlowError(c, err)
		return
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}

func (h *PaymentFlowHandler) Pay(c *gin.Context) {
	var request payRequest

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(
			http.StatusBadRequest,
			gin.H{"error": "channel_code is required"},
		)

		return
	}

	result, err := h.service.Pay(c.Request.Context(), c.Param("order_id"), c.GetString("customer_id"), request.ChannelCode)

	if err != nil {
		respondPaymentFlowError(c, err)
		return
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}

func respondPaymentFlowError(
	c *gin.Context,
	err error,
) {
	switch {
	case errors.Is(err, order.ErrOrderNotFound):
		c.JSON(
			http.StatusNotFound,
			gin.H{"error": "order not found"},
		)

	case errors.Is(err, order.ErrCheckoutRequired):
		c.JSON(
			http.StatusConflict,
			gin.H{"error": "checkout required"},
		)

	case errors.Is(err, order.ErrUnsupportedChannel):
		c.JSON(
			http.StatusBadRequest,
			gin.H{"error": "unsupported channel"},
		)

	case errors.Is(err, order.ErrOrderNotPayable):
		c.JSON(
			http.StatusConflict,
			gin.H{"error": "order cannot be paid"},
		)

	case errors.Is(err, order.ErrCheckoutPaymentMismatch):
		c.JSON(
			http.StatusBadGateway,
			gin.H{"error": "payment data mismatch"},
		)

	case errors.Is(err, order.ErrPaymentProcessingRejected):
		c.JSON(
			http.StatusUnprocessableEntity,
			gin.H{"error": "payment processing rejected"},
		)

	case errors.Is(err, order.ErrPaymentProcessingUncertain),
		errors.Is(err, order.ErrPaymentServiceUnavailable):
		c.JSON(
			http.StatusServiceUnavailable,
			gin.H{"error": "payment status temporarily unavailable"},
		)

	default:
		c.JSON(
			http.StatusInternalServerError,
			gin.H{"error": "internal server error"},
		)
	}
}
