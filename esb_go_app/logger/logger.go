package logger

import (
	"log/slog"
	"os"
	"path/filepath"
)

// New создает и настраивает экземпляр логгера slog.
func New(logDir string) (*slog.Logger, error) {
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

	// Создаем JSON-обработчик, который будет писать в файл
	handler := slog.NewJSONHandler(logFile, &slog.HandlerOptions{
		Level:     slog.LevelInfo, // Уровень логирования (Info, Debug, Warn, Error)
		AddSource: true,           // Добавлять в лог информацию о файле и строке кода
	})

	// Создаем и возвращаем новый логгер
	logger := slog.New(handler)
	return logger, nil
}
