package main

import (
	"context"
	"fmt"
	stdlog "log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli/v2"

	"molly-server/internal/infrastructure/config"
	"molly-server/internal/infrastructure/persistence"
	"molly-server/internal/presentation/http"
	"molly-server/pkg/auth"
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
				Value:   "configs/config.toml",
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
		stdlog.Fatal(err)
	}
}

func cmdServer() *cli.Command {
	return &cli.Command{
		Name:  "server",
		Usage: "Start the web server",
		Action: func(ctx *cli.Context) error {
			cfg := config.MustLoad(ctx.String("config"))
			log := mustInitLogger(cfg)

			auth.Init(cfg.Auth.Secret)

			db := mustInitDB(cfg, log)

			srv := http.NewServer(cfg, db, log)
			return runUntilSignal(srv.Start, srv.Shutdown, log)
		},
	}
}

func cmdMigrate() *cli.Command {
	return &cli.Command{
		Name:  "migrate",
		Usage: "Run database schema migrations",
		Action: func(cliCtx *cli.Context) error {
			cfg := config.MustLoad(cliCtx.String("config"))
			log := mustInitLogger(cfg)
			db := mustInitDB(cfg, log)
			defer func() {
				if err := db.Close(); err != nil {
					log.Error("close db", "error", err)
				}
			}()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			return persistence.Migrate(ctx, db, log)
		},
	}
}

func mustInitLogger(cfg *config.Config) *logger.Logger {
	l, err := logger.New(cfg.Log)
	if err != nil {
		// parseLevel 返回的是非致命降级错误，打印 warn 后继续
		stdlog.Printf("warn: logger: %v", err)
	}
	return l
}

func mustInitDB(cfg *config.Config, log *logger.Logger) *persistence.DB {
	db, err := persistence.Open(cfg.Database, log)
	if err != nil {
		log.Error("database init failed", "error", err)
		os.Exit(1)
	}
	return db
}

func runUntilSignal(start func() error, shutdown func(ctx context.Context) error, log *logger.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- start()
	}()

	select {
	case err := <-errCh:
		// server internal error
		return err
	case <-ctx.Done():
		// receive exit signal
		log.Info("shutting down gracefully...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := shutdown(shutdownCtx); err != nil {
			log.Error("shutdown error", "error", err)
			return fmt.Errorf("shutdown: %w", err)
		}
		return nil
	}
}
