package rabbitmq

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"esb-go-app/config"
	"esb-go-app/scripting"
	"esb-go-app/storage"

	"github.com/rabbitmq/amqp091-go"
)

// RabbitMQ holds the connection and configuration for RabbitMQ interactions.
type RabbitMQ struct {
	conn             *amqp091.Connection
	logger           *slog.Logger
	dataStore        *storage.Store
	scriptingService *scripting.Service
	workers          map[string]bool
	stoppers         map[string]context.CancelFunc // Map to hold cancellation functions for workers
	stoppersMu       sync.Mutex                    // Mutex to protect the stoppers map
	cfg              *config.RabbitMQConfig
}

// New creates a new RabbitMQ instance and connects to the broker.
func New(cfg *config.RabbitMQConfig, logger *slog.Logger, dataStore *storage.Store, scriptingService *scripting.Service) (*RabbitMQ, error) {
	conn, err := amqp091.Dial(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}
	logger.Info("connected to RabbitMQ successfully")

	return &RabbitMQ{
		conn:             conn,
		logger:           logger,
		dataStore:        dataStore,
		scriptingService: scriptingService,
		workers:          make(map[string]bool),
		stoppers:         make(map[string]context.CancelFunc), // Initialize stoppers
		cfg:              cfg,
	}, nil
}

// Close
func (r *RabbitMQ) Close() error {
	return r.conn.Close()
}

// StopRouter stops a running router worker.
func (r *RabbitMQ) StopRouter(routeID string) {
	workerKey := "router-" + routeID

	r.stoppersMu.Lock()
	defer r.stoppersMu.Unlock()

	if cancel, ok := r.stoppers[workerKey]; ok {
		r.logger.Info("stopping router worker", "route_id", routeID)
		cancel() // Signal the worker to stop
		delete(r.stoppers, workerKey)
		delete(r.workers, workerKey)
	}
}

// RestartRouter stops and then starts a router worker.
func (r *RabbitMQ) RestartRouter(routeID, routeName, sourceID string) {
	r.StopRouter(routeID)
	// Give it a moment to shutdown before restarting
	time.Sleep(100 * time.Millisecond)
	r.StartRouter(routeID, routeName, sourceID)
}
