package http

import (
	"github.com/gin-gonic/gin"

	appfile "molly-server/internal/application/file"
	apprecycled "molly-server/internal/application/recycled"
	appuser "molly-server/internal/application/user"
	"molly-server/internal/infrastructure/config"
	"molly-server/internal/infrastructure/persistence"
	"molly-server/internal/presentation/http/handler"
	"molly-server/internal/presentation/http/middleware"
	"molly-server/pkg/cache"
	"molly-server/pkg/logger"
)

func newRouter(cfg *config.Config, db *persistence.DB, c cache.Cache, log *logger.Logger, recycledUC *apprecycled.UseCase) *gin.Engine {
	setGinMode(cfg.App.Env)

	r := gin.New()

	r.Use(
		middleware.Logger(log),
		middleware.Recovery(log),
		middleware.CORS(cfg.Cors),
	)

	userRepo := persistence.NewUserRepo(db.Client)

	userUC := appuser.NewUseCase(userRepo, cfg.Auth)
	fileUC := appfile.NewUseCase(appfile.Deps{
		FileInfo:       persistence.NewFileInfoRepo(db.Client),
		UserFile:       persistence.NewUserFileRepo(db.Client),
		VirtualPath:    persistence.NewVirtualPathRepo(db.Client),
		UploadTask:     persistence.NewUploadTaskRepo(db.Client),
		UserRepo:       userRepo,
		Cache:          c,
		StoragePath:    cfg.Storage.Local.DataDir,
		MoveToRecycled: recycledUC.MoveToRecycled, // 函数注入，解耦域依赖
	})

	userH := handler.NewUserHandler(userUC)
	fileH := handler.NewFileHandler(fileUC)
	recycledH := handler.NewRecycledHandler(recycledUC)

	authM := middleware.NewAuthMiddleware(cfg.Auth, c, userUC)

	api := r.Group("/api")

	userH.RegisterPublic(api)

	authed := api.Group("", authM.Verify())
	{
		userH.Register(authed)     // GET  /api/users/me
		fileH.Register(authed)     // POST /api/files/precheck, GET /api/files ...
		recycledH.Register(authed) // GET  /api/recycled, POST /api/recycled/restore ...
	}

	return r
}

func setGinMode(env string) {
	if env == "production" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}
}
