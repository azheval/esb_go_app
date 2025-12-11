package scripting

import (
	"fmt"
	"log/slog"

	"esb-go-app/storage"
)

// Service manages the execution of different scripting engines.
type Service struct {
	gojaRunner     *GojaRunner
	starlarkRunner *StarlarkRunner
	logger         *slog.Logger
	store          *storage.Store
}

// NewService creates a new scripting service.
func NewService(logger *slog.Logger, httpClient *HTTPClient, store *storage.Store) *Service {
	return &Service{
		gojaRunner:     NewGojaRunner(logger, httpClient, store),
		starlarkRunner: NewStarlarkRunner(logger, httpClient, store),
		logger:         logger,
		store:          store,
	}
}

// ExecuteScript executes a script using the specified engine.
func (s *Service) ExecuteScript(
	engine string,
	script string,
	messageBody map[string]interface{},
	messageHeaders map[string]interface{},
) (*TransformedMessage, error) {
	switch engine {
	case "javascript":
		return s.gojaRunner.Execute(script, messageBody, messageHeaders)
	case "starlark":
		return s.starlarkRunner.Execute(script, messageBody, messageHeaders)
	default:
		return nil, fmt.Errorf("unsupported scripting engine: %s", engine)
	}
}
