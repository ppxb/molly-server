package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"molly-server/pkg/logger"
)

func Recovery(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				if ne, ok := err.(error); ok && ne.Error() == "http: abort handler" {
					return
				}

				log.Error("panic recovered",
					"error", err,
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"ip", c.ClientIP(),
					"uri", c.Request.RequestURI,
				)

				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"code":    500,
					"message": "internal server error",
				})
			}
		}()
		c.Next()
	}
}
