package api

import (
	"github.com/gin-gonic/gin"

	apiv1 "molly-server/internal/api/v1"
)

type Dependencies struct {
	V1 apiv1.Dependencies
}

func Register(engine *gin.Engine, deps Dependencies) {
	v1Group := engine.Group("/v1")
	apiv1.Register(v1Group, deps.V1)
}
