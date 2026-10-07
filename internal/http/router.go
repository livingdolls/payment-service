package http

import (
	"context"
	nethttp "net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/livingdolls/payment-service/internal/http/handler"
)

func NewRouter(db *pgxpool.Pool, paymentHandler *handler.PaymentHandler) *gin.Engine {
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

	v1 := router.Group("/v1")

	payments := v1.Group("/payments")
	attempts := v1.Group("/payment-attempts")

	payments.POST("", paymentHandler.Create)

	attempts.POST("/:attempt_id/process", paymentHandler.Process)

	return router
}
