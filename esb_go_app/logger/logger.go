package logger

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// New создает и настраивает экземпляр логгера slog.
func New(logDir, version, logLevel string) (*slog.Logger, error) {
	// Создаем директорию, если она не существует
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}

	// Создаем или открываем файл для логов
	logPath := filepath.Join(logDir, "esb_go_app.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
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

	// Создаем JSON-обработчик, который будет писать в файл
	handler := slog.NewJSONHandler(logFile, &slog.HandlerOptions{
		Level:     level, // Уровень логирования (Info, Debug, Warn, Error)
		AddSource: true,  // Добавлять в лог информацию о файле и строке кода
	})

	// Создаем логгер и добавляем в него постоянный атрибут "version"
	logger := slog.New(handler).With("version", version)
	return logger, nil
}
