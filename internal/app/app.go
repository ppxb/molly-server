package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"molly-server/ent"
	projectapi "molly-server/internal/api"
	apiv1 "molly-server/internal/api/v1"
	"molly-server/internal/config"
	"molly-server/internal/platform/database"
	"molly-server/internal/platform/httpserver"
	uploadapi "molly-server/internal/upload/api"
	"molly-server/internal/upload/repository"
	"molly-server/internal/upload/service"
	"molly-server/pkg/objectstorage"
)

type App struct {
	cfg        config.Config
	logger     *slog.Logger
	entClient  *ent.Client
	httpServer *http.Server
}

func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	dbClient, err := database.NewEntClient(context.Background(), cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("new app: init database: %w", err)
	}

	uploadRepo := repository.New(dbClient)
	storageClient, err := objectstorage.New(cfg.ObjectStorage)
	if err != nil {
		return nil, fmt.Errorf("new app: init object storage: %w", err)
	}

	uploadService := service.New(uploadRepo, cfg.Upload, cfg.ObjectStorage, storageClient)
	uploadHandler := uploadapi.NewHandler(uploadService, logger)

	apiDeps := projectapi.Dependencies{
		V1: apiv1.Dependencies{
			PublicRouters: []apiv1.PublicRouter{uploadHandler},
		},
	}

	router := httpserver.NewRouter(cfg, logger, httpserver.Dependencies{API: apiDeps})
	httpServer := httpserver.New(cfg.HTTP, router)

	return &App{
		cfg:        cfg,
		logger:     logger,
		entClient:  dbClient,
		httpServer: httpServer,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	serverErr := make(chan error, 1)

	go func() {
		a.logger.Info("http server started", "addr", a.httpServer.Addr)
		if err := a.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	var runErr error
	select {
	case <-ctx.Done():
		a.logger.Info("shutdown signal received")
	case err := <-serverErr:
		if err != nil {
			runErr = fmt.Errorf("http server: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(a.cfg.HTTP.ShutdownTimeoutSeconds)*time.Second)
	defer cancel()

	if err := a.httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		if runErr == nil {
			runErr = fmt.Errorf("shutdown http server: %w", err)
		}
	}

	if err := a.entClient.Close(); err != nil {
		a.logger.Error("close database client failed", "error", err)
		if runErr == nil {
			runErr = fmt.Errorf("close database: %w", err)
		}
	}

	return runErr
}
