package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v2"

	"molly-server/internal/infrastructure/config"
	"molly-server/pkg/logger"
)

// @title           Molly API
// @version         1.0
// @description     Enterprise cloud drive with native S3 and MinIO support.
// @host            localhost:9527
// @BasePath        /api
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

func main() {
	app := &cli.App{
		Name:  "molly",
		Usage: "Enterprise cloud drive — supports S3, MinIO, and WebDAV",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Value:   "config/config.toml",
				Usage:   "Path to config file",
				EnvVars: []string{"MOLLY_CONFIG"},
			},
		},
		Commands: []*cli.Command{
			cmdServer(),
			cmdMigrate(),
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func cmdServer() *cli.Command {
	return &cli.Command{
		Name:  "server",
		Usage: "Start the web server",
		Action: func(ctx *cli.Context) error {
			cfg := config.MustLoad(ctx.String("config"))
			_ = mustInitLogger(cfg)
			return nil
		},
	}
}

func cmdMigrate() *cli.Command {
	return &cli.Command{
		Name:  "migrate",
		Usage: "Run database migrations",
		Action: func(ctx *cli.Context) error {
			cfg := config.MustLoad(ctx.String("config"))
			l := mustInitLogger(cfg)

			l.Info("running migrations...")
			return nil
		},
	}
}

func mustInitLogger(cfg *config.Config) *logger.Logger {
	l, err := logger.New(cfg.Log)
	if err != nil {
		log.Fatalf("logger init: %v", err)
	}
	return l
}

func runUntilSignal(start func() error, shutdown func(ctx context.Context) error, log *logger.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- start() }()

	select {
	case err := <-errCh:
		// server internal error
		return err
	case <-ctx.Done():
		// receive exit signal
		log.Info("shutting down gracefully...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30e9)
		defer cancel()
		if err := shutdown(shutdownCtx); err != nil {
			log.Error("shutdown error", "error", err)
			return err
		}
		log.Info("server stopped")
		return nil
	}
}
