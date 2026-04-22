package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"molly-server/internal/infrastructure/config"
)

func CORS(cfg config.CorsConfig) gin.HandlerFunc {
	allowHeaders := strings.Join([]string{
		"Content-Type", "Authorization",
		"X-API-Key", "X-Signature", "X-Timestamp", "X-Nonce", "X-Requested-With",
		cfg.AllowHeaders,
	}, ", ")

	return func(c *gin.Context) {
		if !cfg.Enable {
			c.Next()
			return
		}

		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			c.Next()
			return
		}

		if cfg.AllowOrigins == "*" {
			c.Header("Access-Control-Allow-Origin", "*")
		} else {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}

		c.Header("Access-Control-Allow-Headers", allowHeaders)
		c.Header("Access-Control-Allow-Methods", cfg.AllowMethods)
		c.Header("Access-Control-Expose-Headers", cfg.ExposeHeaders)
		c.Header("Access-Control-Max-Age", "86400")

		if cfg.AllowCredentials {
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
