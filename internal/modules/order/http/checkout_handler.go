package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/livingdolls/payment-service/internal/modules/order"
)

type CheckoutHandler struct {
	service *order.CheckoutService
}

func NewCheckoutHandler(service *order.CheckoutService) *CheckoutHandler {
	return &CheckoutHandler{
		service: service,
	}
}

func (h *CheckoutHandler) Start(c *gin.Context) {
	orderID := strings.TrimSpace(c.Param("order_id"))

	if orderID == "" {
		c.JSON(
			http.StatusBadRequest,
			gin.H{"error": "order_id is required"},
		)

		return
	}

	result, err := h.service.Start(c.Request.Context(), orderID, c.GetString("customer_id"))

	if err != nil {
		switch {
		case errors.Is(err, order.ErrOrderNotFound):
			c.JSON(
				http.StatusNotFound,
				gin.H{"error": "order not found"},
			)

		case errors.Is(err, order.ErrOrderNotPayable),
			errors.Is(err, order.ErrCheckoutAlreadyBound),
			errors.Is(err, order.ErrCheckoutOrderChanged):

			c.JSON(
				http.StatusConflict,
				gin.H{"error": err.Error()},
			)

		case errors.Is(err, order.ErrCheckoutPaymentMismatch):
			c.JSON(
				http.StatusBadGateway,
				gin.H{"error": "payment response missmatch"},
			)

		case errors.Is(err, order.ErrPaymentServiceUnavailable):
			c.JSON(
				http.StatusServiceUnavailable,
				gin.H{
					"error":     "payment service unavailable",
					"retryable": true,
				},
			)

		default:
			c.JSON(
				http.StatusInternalServerError,
				gin.H{"error": "checkout failed"},
			)
		}

		return
	}

	c.JSON(http.StatusOK, result)
}
