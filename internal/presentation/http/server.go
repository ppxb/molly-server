package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	apprecycled "molly-server/internal/application/recycled"
	"molly-server/internal/infrastructure/config"
	"molly-server/internal/infrastructure/persistence"
	"molly-server/pkg/cache"
	"molly-server/pkg/logger"
)

type Server struct {
	db   *persistence.DB
	log  *logger.Logger
	http *http.Server
}

// NewServer 构造 Server，同时完成路由注册
func NewServer(cfg *config.Config,
	db *persistence.DB,
	c cache.Cache,
	log *logger.Logger,
	recycledUC *apprecycled.UseCase) *Server {
	engine := newRouter(cfg, db, c, log, recycledUC)

	return &Server{
		db:  db,
		log: log,
		http: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
			Handler:      engine,
			ReadTimeout:  cfg.Server.ReadTimeout,
			WriteTimeout: cfg.Server.WriteTimeout,
		},
	}
}

func (s *Server) Start() error {
	s.log.Info("server listening", "addr", s.http.Addr)

	if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server listening failed: %w", err)
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.log.Info("server shutting down...")

	if err := s.http.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	if err := s.db.Close(); err != nil {
		s.log.Error("close db", "error", err)
	}

	s.log.Info("server stopped")
	return nil
}
