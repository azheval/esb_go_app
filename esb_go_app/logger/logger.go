package logger

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lestrrat-go/file-rotatelogs"
)

func New(logDir, version, logLevel string) (*slog.Logger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}

	logPattern := filepath.Join(logDir, "esb_go_app-%Y-%m-%d-%H.log")
	logf, err := rotatelogs.New(
		logPattern,
		rotatelogs.WithMaxAge(7*24*time.Hour),
		rotatelogs.WithRotationTime(time.Hour),
	)
	if err != nil {
		return nil, err
	}

	var level slog.Level
	switch strings.ToLower(logLevel) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(logf, &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	})

	logger := slog.New(handler).With("version", version)
	return logger, nil
}
