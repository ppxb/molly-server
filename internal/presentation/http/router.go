package http

import (
	"github.com/gin-gonic/gin"

	"molly-server/internal/infrastructure/config"
	"molly-server/internal/infrastructure/persistence"
	"molly-server/pkg/logger"
)

func newRouter(cfg *config.Config, db *persistence.DB, log *logger.Logger) *gin.Engine {
	setGinMode(cfg.App.Env)

	r := gin.New()

	return r
}

func setGinMode(env string) {
	if env == "production" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}
}
