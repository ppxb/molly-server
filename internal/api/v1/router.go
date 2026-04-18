package v1

import "github.com/gin-gonic/gin"

type PublicRouter interface {
	RegisterPublicRoutes(group *gin.RouterGroup)
}

type AuthRouter interface {
	RegisterAuthRoutes(group *gin.RouterGroup)
}

type Dependencies struct {
	PublicRouters []PublicRouter
	AuthRouters   []AuthRouter
	AuthHandlers  []gin.HandlerFunc
}

func Register(group *gin.RouterGroup, deps Dependencies) {
	publicGroup := group.Group("")
	for _, router := range deps.PublicRouters {
		router.RegisterPublicRoutes(publicGroup)
	}

	handlers := make([]gin.HandlerFunc, 0, len(deps.AuthHandlers))
	handlers = append(handlers, deps.AuthHandlers...)
	authGroup := group.Group("", handlers...)
	for _, router := range deps.AuthRouters {
		router.RegisterAuthRoutes(authGroup)
	}
}
