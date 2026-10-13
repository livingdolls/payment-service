package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

func RequireServiceToken(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		expected := "Bearer " + token
		actual := c.GetHeader("Authorization")

		if len(token) < 32 || subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
			c.AbortWithStatusJSON(
				http.StatusUnauthorized,
				gin.H{
					"error": gin.H{
						"code":    "UNAUTHORIZED",
						"message": "unauthorized",
					},
				},
			)

			return
		}

		c.Next()
	}
}
