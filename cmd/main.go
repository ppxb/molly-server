package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"molly-server/internal/app"
	"molly-server/internal/config"
	"molly-server/internal/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	log := logger.New(cfg.Log.Level)
	application, err := app.New(cfg, log)
	if err != nil {
		log.Error("failed to initialize application", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := application.Run(ctx); err != nil {
		log.Error("application exited with error", "error", err)
		os.Exit(1)
	}
}
