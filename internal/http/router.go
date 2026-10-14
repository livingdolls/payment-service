package http

import (
	"context"
	nethttp "net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/livingdolls/payment-service/internal/http/handler"
	"github.com/livingdolls/payment-service/internal/http/middleware"
)

func NewRouter(db *pgxpool.Pool, paymentHandler *handler.PaymentHandler, webhookHandler *handler.WebhookHandler, reviewHandler *handler.ReconciliationReviewHandler, adminToken string, adminActor string, paymentInternalToken string) *gin.Engine {
	router := gin.New()

	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	router.GET("/health/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			c.JSON(nethttp.StatusServiceUnavailable, gin.H{
				"status":   "unhealthy",
				"database": "down",
			})

			return
		}

		c.JSON(nethttp.StatusOK, gin.H{
			"status":   "ok",
			"database": "healthy",
		})
	})

	paymentStatusHandler := handler.NewPaymentStatusHandler(db)

	v1 := router.Group("/v1")
	webhooks := router.Group("/webhooks")
	admin := router.Group("/admin", middleware.RequireAdmin(adminToken, adminActor))
	internal := router.Group("/internal", middleware.RequireServiceToken(paymentInternalToken))

	payments := v1.Group("/payments")
	attempts := v1.Group("/payment-attempts")
	xenditWebhooks := webhooks.Group("/xendit")

	payments.POST("", paymentHandler.Create)

	attempts.POST("/:attempt_id/process", paymentHandler.Process)
	xenditWebhooks.POST("/payments", webhookHandler.XenditPayment)

	admin.GET("/reconciliation-reviews", reviewHandler.List)
	admin.POST("/payment-attempts/:attempt_id/requeue-reconciliation", reviewHandler.Requeue)

	internal.POST("/payments", paymentHandler.Create)
	internal.GET("/payments/:payment_id/status", paymentStatusHandler.Get)
	internal.POST("/payment-attempts/:attempt_id/process", paymentHandler.Process)

	return router
}
