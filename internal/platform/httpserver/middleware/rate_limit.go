package middleware

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"molly-server/internal/config"
	"molly-server/internal/platform/httpserver/response"
)

func RateLimit(cfg config.RateLimitConfig, log *slog.Logger) gin.HandlerFunc {
	if !cfg.Enabled {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	limiter := rate.NewLimiter(rate.Limit(cfg.RequestsPerSecond), cfg.Burst)
	return func(c *gin.Context) {
		if limiter.Allow() {
			c.Next()
			return
		}

		requestID, _ := c.Get(requestIDContextKey)
		log.Warn("rate limit exceeded", "request_id", requestID, "path", c.FullPath(), "client_ip", c.ClientIP())
		response.Error(c, 429, "TooManyRequests", "rate limit exceeded")
	}
}
