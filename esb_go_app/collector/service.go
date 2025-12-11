package collector

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"esb-go-app/rabbitmq"
	"esb-go-app/scripting"
	"esb-go-app/storage"
)

// Service is responsible for running collectors.
type Service struct {
	store     *storage.Store
	scripting *scripting.Service
	rmq       *rabbitmq.RabbitMQ
	logger    *slog.Logger
}

// NewService creates a new collector service.
func NewService(store *storage.Store, scripting *scripting.Service, rmq *rabbitmq.RabbitMQ, logger *slog.Logger) *Service {
	return &Service{
		store:     store,
		scripting: scripting,
		rmq:       rmq,
		logger:    logger,
	}
}

// RunCollector executes a single collector job.
func (s *Service) RunCollector(collectorID string) {
	s.logger.Info("running collector", "collector_id", collectorID)

	collector, err := s.store.GetCollectorByID(collectorID)
	if err != nil || collector == nil {
		s.logger.Error("failed to get collector for execution", "collector_id", collectorID, "error", err)
		return
	}

	// Execute the script
	transformedMsg, err := s.scripting.ExecuteScript(collector.Engine, collector.Script, nil, nil)
	if err != nil {
		s.logger.Error("failed to execute collector script", "collector_id", collectorID, "error", err)
		return
	}

	if transformedMsg == nil || transformedMsg.Body == nil {
		s.logger.Info("collector script did not return any data", "collector_id", collectorID)
		return
	}

	// Marshal the message body to JSON
	bodyBytes, err := json.Marshal(transformedMsg.Body)
	if err != nil {
		s.logger.Error("failed to marshal collector message body to JSON", "collector_id", collectorID, "error", err)
		return
	}

	// The destination is now an internal exchange unique to the collector
	exchangeName := fmt.Sprintf("collector-output:%s", collector.ID)

	if err := s.rmq.EnsureExchange(exchangeName); err != nil {
		s.logger.Error("failed to ensure collector output exchange exists", "collector_id", collectorID, "exchange", exchangeName, "error", err)
		return
	}

	// Publish the message to the collector's own output exchange
	err = s.rmq.Publish(exchangeName, "", string(bodyBytes))
	if err != nil {
		s.logger.Error("failed to publish collected message", "collector_id", collectorID, "exchange", exchangeName, "error", err)
		return
	}

	s.logger.Info("collector successfully executed and message published", "collector_id", collectorID, "exchange", exchangeName)
}
