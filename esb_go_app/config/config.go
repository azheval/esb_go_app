package config

import (
	"encoding/json"
	"os"
)

// RabbitMQConfig содержит конфигурацию для RabbitMQ.
type RabbitMQConfig struct {
	DSN string `json:"dsn"`
}

// Config представляет структуру файла конфигурации.
type Config struct {
	Port     string         `json:"port"`
	LogDir   string         `json:"log_dir"`
	DBPath   string         `json:"db_path"`
	LogLevel string         `json:"log_level"`
	RabbitMQ RabbitMQConfig `json:"rabbitmq"`
}

// Load загружает конфигурацию из указанного файла.
func Load(filePath string) (*Config, error) {
	cfg := &Config{
		Port:     "8080",
		LogDir:   "logs",
		DBPath:   "data/esb.db",
		LogLevel: "info", // Default log level
		RabbitMQ: RabbitMQConfig{
			DSN: "amqp://guest:guest@localhost:5672/",
		},
	}

	file, err := os.Open(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else {
		defer file.Close()
		decoder := json.NewDecoder(file)
		if err := decoder.Decode(cfg); err != nil {
			return nil, err
		}
	}

	if dsn := os.Getenv("RABBITMQ_DSN"); dsn != "" {
		cfg.RabbitMQ.DSN = dsn
	}

	return cfg, nil
}
