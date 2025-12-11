package scripting

import (
	"fmt"
	"log/slog"

	"esb-go-app/storage"

	"github.com/dop251/goja"
)

// GojaRunner implements the Runner interface for JavaScript scripts using Goja.
type GojaRunner struct {
	logger     *slog.Logger
	httpClient *HTTPClient // Injected HTTP client
	store      *storage.Store
}

// NewGojaRunner creates a new GojaRunner instance.
func NewGojaRunner(logger *slog.Logger, httpClient *HTTPClient, store *storage.Store) *GojaRunner {
	return &GojaRunner{
		logger:     logger,
		httpClient: httpClient,
		store:      store,
	}
}

// Execute runs the JavaScript script.
func (r *GojaRunner) Execute(script string, messageBody map[string]interface{}, messageHeaders map[string]interface{}) (*TransformedMessage, error) {
	vm := goja.New()

	vm.Set("log", NewLogger(r.logger))
	vm.Set("http", r.httpClient)

	jsBody := vm.ToValue(messageBody)
	jsHeaders := vm.ToValue(messageHeaders)

	program, err := goja.Compile("script", script, false)
	if err != nil {
		return nil, fmt.Errorf("failed to compile JavaScript script: %w", err)
	}
	_, err = vm.RunProgram(program)
	if err != nil {
		return nil, fmt.Errorf("failed to run JavaScript script: %w", err)
	}

	// Handle 'transform' function for transformation routes
	if transformFunc, ok := goja.AssertFunction(vm.Get("transform")); ok {
		result, err := transformFunc(goja.Undefined(), jsBody, jsHeaders)
		if err != nil {
			return nil, fmt.Errorf("failed to execute transform function: %w", err)
		}

		// Handle script returning null to filter message
		if goja.IsNull(result) || goja.IsUndefined(result) {
			return nil, nil // Indicate that the message should be dropped
		}

		var resultObj map[string]interface{}
		if err := vm.ExportTo(result, &resultObj); err != nil {
			return nil, fmt.Errorf("failed to export transform result: %w", err)
		}

		// The script is only responsible for the body now. Destination is ignored.
		transformedBody, _ := resultObj["body"].(map[string]interface{})

		// If body is not returned, treat as null/filter
		if transformedBody == nil {
			return nil, nil
		}

		return &TransformedMessage{
			Body:    transformedBody,
			Headers: messageHeaders, // Headers are passed through for now
		}, nil
	}

	// Handle 'collect' function for collector jobs
	if collectFunc, ok := goja.AssertFunction(vm.Get("collect")); ok {
		result, err := collectFunc(goja.Undefined())
		if err != nil {
			return nil, fmt.Errorf("failed to execute collect function: %w", err)
		}

		if goja.IsNull(result) || goja.IsUndefined(result) {
			return nil, nil // No data collected
		}

		// For now, we only support returning a single message object from a collector script.
		var resultObj map[string]interface{}
		if err := vm.ExportTo(result, &resultObj); err != nil {
			return nil, fmt.Errorf("failed to export collect result into a message object: %w", err)
		}

		if len(resultObj) > 0 {
			return &TransformedMessage{
				Body:    resultObj,
				Headers: make(map[string]interface{}), // Collectors start with fresh headers
			}, nil
		}
		return nil, nil // No data in the object
	}

	return nil, fmt.Errorf("script must define a 'transform' or 'collect' function")
}
