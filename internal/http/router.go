package http

import (
	"context"
	nethttp "net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewRouter(db *pgxpool.Pool) *gin.Engine {
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

	return router
}
