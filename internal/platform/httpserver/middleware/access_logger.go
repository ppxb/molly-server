package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

func AccessLogger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		startAt := time.Now()
		path := c.Request.URL.Path
		rawQuery := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(startAt)
		if rawQuery != "" {
			path = path + "?" + rawQuery
		}

		requestID, _ := c.Get(requestIDContextKey)
		log.Info("request completed",
			"request_id", requestID,
			"method", c.Request.Method,
			"path", path,
			"status", c.Writer.Status(),
			"latency_ms", latency.Milliseconds(),
			"client_ip", c.ClientIP(),
		)
	}
}
