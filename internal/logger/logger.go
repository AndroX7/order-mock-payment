// Package logger builds the application's slog.Logger.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// New returns a slog.Logger configured for the environment.
//   - production  -> JSON handler (structured, ingest-friendly)
//   - anything else -> text handler (human-readable)
func New(env, level string) *slog.Logger {
	isProd := strings.EqualFold(env, "production")
	opts := &slog.HandlerOptions{
		Level:     parseLevel(level),
		AddSource: isProd,
	}

	var h slog.Handler
	if isProd {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(h)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
