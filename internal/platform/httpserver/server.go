package httpserver

import (
	"net/http"
	"time"

	"molly-server/internal/config"
)

func New(cfg config.HTTPConfig, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         cfg.Address(),
		Handler:      handler,
		ReadTimeout:  time.Duration(cfg.ReadTimeoutSeconds) * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:  time.Duration(cfg.IdleTimeoutSeconds) * time.Second,
	}
}
