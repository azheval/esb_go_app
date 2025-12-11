package config

import (
	"encoding/json"
	"os"
)

type RabbitMQConfig struct {
	DSN            string `json:"dsn"`
	ManagementDSN  string `json:"management_dsn"`
	ManagementUser string `json:"management_user"`
	ManagementPass string `json:"management_pass"`
}

type Config struct {
	Port     string         `json:"port"`
	LogDir   string         `json:"log_dir"`
	DBPath   string         `json:"db_path"`
	LogLevel string         `json:"log_level"`
	RabbitMQ RabbitMQConfig `json:"rabbitmq"`
}

func Load(filePath string) (*Config, error) {
	cfg := &Config{
		Port:     "8080",
		LogDir:   "logs",
		DBPath:   "data/esb.db",
		LogLevel: "info",
		RabbitMQ: RabbitMQConfig{
			DSN:            "amqp://guest:guest@rabbitmq:5672/",
			ManagementDSN:  "http://rabbitmq:15672",
			ManagementUser: "guest",
			ManagementPass: "guest",
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
	if mdsn := os.Getenv("RABBITMQ_MANAGEMENT_DSN"); mdsn != "" {
		cfg.RabbitMQ.ManagementDSN = mdsn
	}

	return cfg, nil
}
