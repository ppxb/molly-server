package middleware

import (
	"log/slog"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"molly-server/internal/platform/httpserver/response"
)

func Recovery(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				requestID, _ := c.Get(requestIDContextKey)
				log.Error("panic recovered",
					"request_id", requestID,
					"panic", recovered,
					"stack", string(debug.Stack()),
				)
				response.Error(c, 500, "InternalError", "internal server error")
			}
		}()
		c.Next()
	}
}
