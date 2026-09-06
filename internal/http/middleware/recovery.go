package middleware

import (
	"github.com/example/e-commerce-be/internal/http/response"
	"github.com/gin-gonic/gin"
	"log/slog"
)

// Recovery does not expose panic values or request headers (which may contain credentials).
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recover() != nil {
				slog.Error("request handler panicked")
				if c.Writer.Written() {
					c.Abort()
					return
				}
				response.Abort(c, 500, "internal server error")
			}
		}()
		c.Next()
	}
}
