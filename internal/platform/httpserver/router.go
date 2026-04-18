package httpserver

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	projectapi "molly-server/internal/api"
	"molly-server/internal/config"
	"molly-server/internal/platform/httpserver/middleware"
)

type Dependencies struct {
	API projectapi.Dependencies
}

func NewRouter(cfg config.Config, log *slog.Logger, deps Dependencies) *gin.Engine {
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()
	engine.Use(
		middleware.CORS(),
		middleware.RequestID(),
		middleware.Recovery(log),
		middleware.AccessLogger(log),
		middleware.RateLimit(cfg.RateLimit, log),
	)

	engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
			"time":   time.Now().UTC().Format(time.RFC3339),
		})
	})

	projectapi.Register(engine, deps.API)
	return engine
}
