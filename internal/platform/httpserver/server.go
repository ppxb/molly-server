package httpserver

import (
	"fmt"
	"net/http"
	"time"

	"molly-server/internal/config"
)

func New(cfg config.HTTPConfig, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:      handler,
		ReadTimeout:  time.Duration(cfg.ReadTimeoutSeconds) * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:  time.Duration(cfg.IdleTimeoutSeconds) * time.Second,
	}
}
