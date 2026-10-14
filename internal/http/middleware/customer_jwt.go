package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func RequireCustoemrJWT(secret string, issuer string, audience string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authorization := c.GetHeader("Authorization")

		if !strings.HasPrefix(authorization, "Bearer ") {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		rawToken := strings.TrimPrefix(authorization, "Bearer ")

		claims := &jwt.RegisteredClaims{}

		token, err := jwt.ParseWithClaims(
			rawToken,
			claims,
			func(t *jwt.Token) (any, error) {
				return []byte(secret), nil
			},
			jwt.WithValidMethods([]string{"HS256"}),
			jwt.WithIssuer(issuer),
			jwt.WithAudience(audience),
			jwt.WithExpirationRequired(),
		)

		if err != nil || !token.Valid || claims.Subject == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		c.Set("customer_id", claims.Subject)
		c.Next()
	}
}
