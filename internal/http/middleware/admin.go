package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

func RequireAdmin(expectedToken string, actor string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authorization := c.GetHeader("Authorization")

		expected := "Bearer " + expectedToken

		if expectedToken == "" || subtle.ConstantTimeCompare([]byte(authorization), []byte(expected)) != 1 {
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

		// Identitas diambil dari konfigurasi server,
		// bukan header yang bisa dipalsukan client.
		c.Set("admin_actor", actor)

		c.Next()
	}
}
