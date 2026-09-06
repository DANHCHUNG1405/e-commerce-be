package middleware

import (
	"net/http"
	"strings"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/http/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequireAccessToken validates a bearer access token and exposes user_id/role in Gin context.
func RequireAccessToken(tokens coreauth.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		parts := strings.SplitN(c.GetHeader("Authorization"), " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			response.Abort(c, http.StatusUnauthorized, "missing bearer token")
			return
		}
		claims, err := tokens.Parse(parts[1])
		if err != nil || claims.Type != "access" {
			response.Abort(c, http.StatusUnauthorized, "invalid access token")
			return
		}
		if _, err := uuid.Parse(claims.Subject); err != nil {
			response.Abort(c, http.StatusUnauthorized, "invalid access token")
			return
		}
		c.Set("user_id", claims.Subject)
		c.Set("role", claims.Role)
		c.Next()
	}
}
