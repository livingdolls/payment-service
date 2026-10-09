package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/livingdolls/payment-service/internal/application"
	"github.com/livingdolls/payment-service/internal/modules/reconciliation"
)

type ReconciliationReviewHandler struct {
	useCase *application.ReconciliationReviewUseCase
}

type requestRequest struct {
	Reason string `json:"reason" binding:"required"`
}

func NewReconciliationReviewHandler(useCase *application.ReconciliationReviewUseCase) *ReconciliationReviewHandler {
	return &ReconciliationReviewHandler{
		useCase: useCase,
	}
}

func (h *ReconciliationReviewHandler) List(c *gin.Context) {
	limit := 20

	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)

		if err != nil || parsed <= 0 {
			respondError(
				c,
				http.StatusBadRequest,
				"INVALID_LIMIT",
				"limit must be a positive integer",
			)

			return
		}

		limit = parsed
	}

	items, err := h.useCase.List(
		c.Request.Context(),
		limit,
	)

	if err != nil {
		respondError(
			c,
			http.StatusInternalServerError,
			"REVIEW_LIST_FAILED",
			"failed to list reconciliation reviews",
		)

		return
	}

	c.JSON(
		http.StatusOK,
		gin.H{
			"data": items,
		},
	)
}

func (h *ReconciliationReviewHandler) Requeue(c *gin.Context) {
	var request requestRequest

	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(
			c,
			http.StatusBadRequest,
			"INVALID_REQUEST",
			"reason is required",
		)
		return
	}

	attemptID := c.Param("attempt_id")
	actor := c.GetString("admin_actor")

	result, err := h.useCase.Requeue(c.Request.Context(), attemptID, actor, request.Reason)

	if err != nil {
		switch {
		case errors.Is(err, reconciliation.ErrInvalidAttemptID), errors.Is(err, reconciliation.ErrInvalidReason):
			respondError(
				c,
				http.StatusBadRequest,
				"INVALID_REQUEUE_REQUEST",
				err.Error(),
			)

		case errors.Is(err, reconciliation.ErrInvalidActor):
			respondError(
				c,
				http.StatusUnauthorized,
				"INVALID_ADMIN_IDENTITY",
				"invalid admin identity",
			)

		case errors.Is(err, reconciliation.ErrReviewNotFound):
			respondError(
				c,
				http.StatusNotFound,
				"PAYMENT_ATTEMPT_NOT_FOUND",
				"payment attempt not found",
			)

		case errors.Is(err, reconciliation.ErrNotNeedsReview), errors.Is(err, reconciliation.ErrNotEligible):
			respondError(
				c,
				http.StatusConflict,
				"REQUEUE_NOT_ALLOWED",
				err.Error(),
			)

		default:
			respondError(
				c,
				http.StatusInternalServerError,
				"REQUEUE_FAILED",
				"failed to requeue reconciliation",
			)
		}

		return
	}

	c.JSON(
		http.StatusAccepted,
		gin.H{
			"status": "QUEUED",
			"action": result,
		},
	)
}
