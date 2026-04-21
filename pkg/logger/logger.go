package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"

	"molly-server/internal/infrastructure/config"
)

type Logger struct {
	*slog.Logger
}

func New(cfg config.LogConfig) (*Logger, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: level == slog.LevelDebug,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				// 统一为 "2006-01-02 15:04:05"，对所有 handler 生效
				a.Value = slog.StringValue(a.Value.Time().Format(time.DateTime))
			}
			return a
		},
	}

	consoleHandler := slog.NewTextHandler(os.Stdout, opts)

	var handler slog.Handler = consoleHandler

	if cfg.LogPath != "" && cfg.LogPath != "stdout" {
		logDir := filepath.Dir(cfg.LogPath)
		if mkErr := os.MkdirAll(logDir, 0o755); mkErr != nil {
			return nil, fmt.Errorf("logger: mkdir %q: %w", cfg.LogPath, mkErr)
		}

		rotator := &lumberjack.Logger{
			Filename:  cfg.LogPath,
			MaxSize:   cfg.MaxSize,
			MaxAge:    cfg.MaxAge,
			Compress:  true,
			LocalTime: true,
		}

		handler = multiHandler(
			consoleHandler,
			slog.NewJSONHandler(rotator, opts),
		)
	}

	return &Logger{
		slog.New(handler),
	}, nil
}

type fanoutHandler []slog.Handler

func multiHandler(handlers ...slog.Handler) slog.Handler {
	return fanoutHandler(handlers)
}

func (f fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range f {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (f fanoutHandler) Handle(ctx context.Context, r slog.Record) error {
	var first error
	for _, h := range f {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}

func (f fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make(fanoutHandler, len(f))
	for i, h := range f {
		next[i] = h.WithAttrs(attrs)
	}
	return next
}

func (f fanoutHandler) WithGroup(name string) slog.Handler {
	next := make(fanoutHandler, len(f))
	for i, h := range f {
		next[i] = h.WithGroup(name)
	}
	return next
}

func parseLevel(s string) (slog.Level, error) {
	switch s {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("logger: unknown level %q, fallback to info", s)
	}
}
