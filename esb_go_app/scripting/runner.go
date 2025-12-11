package scripting

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"time"
)
// TransformedMessage represents the output of a transformation or collector script.
type TransformedMessage struct {
	Body        map[string]interface{}
	Headers     map[string]interface{}
	Destination string // The destination channel name for routing
}

// Runner defines the interface for executing a script.
// It takes the script code, the message body, and message headers as input.
// It returns a TransformedMessage or an error.
type Runner interface {
	Execute(script string, messageBody map[string]interface{}, messageHeaders map[string]interface{}) (*TransformedMessage, error)
}

// Logger is a simplified logger interface for scripts
type Logger struct {
	*slog.Logger
}

func (l *Logger) Log(level string, msg string, args ...interface{}) {
	switch level {
	case "debug":
		l.Debug(msg, args...)
	case "info":
		l.Info(msg, args...)
	case "warn":
		l.Warn(msg, args...)
	case "error":
		l.Error(msg, args...)
	default:
		l.Info(msg, args...)
	}
}

// NewLogger creates a new script-friendly logger.
func NewLogger(logger *slog.Logger) *Logger {
	return &Logger{logger}
}

// HTTPClient is a wrapper for net/http.Client to be injected into scripts.
type HTTPClient struct {
	Client *http.Client
	Logger *slog.Logger
}

type HTTPResponse struct {
	StatusCode int
	Body       string
	Headers    map[string]string
	Error      string
}

// NewHTTPClient creates a new HTTPClient with a default timeout.
func NewHTTPClient(logger *slog.Logger) *HTTPClient {
	return &HTTPClient{
		Client: &http.Client{
			Timeout: 10 * time.Second, // Default timeout
		},
		Logger: logger,
	}
}

// Get performs an HTTP GET request.
func (c *HTTPClient) Get(url string, headers map[string]string) *HTTPResponse {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		c.Logger.Error("failed to create GET request", "error", err, "url", url)
		return &HTTPResponse{Error: err.Error()}
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		c.Logger.Error("failed to perform GET request", "error", err, "url", url)
		return &HTTPResponse{Error: err.Error()}
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		c.Logger.Error("failed to read response body", "error", err, "url", url)
		return &HTTPResponse{StatusCode: resp.StatusCode, Error: err.Error()}
	}

	respHeaders := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			respHeaders[k] = v[0]
		}
	}

	return &HTTPResponse{
		StatusCode: resp.StatusCode,
		Body:       string(bodyBytes),
		Headers:    respHeaders,
	}
}

// Post performs an HTTP POST request.
func (c *HTTPClient) Post(url string, headers map[string]string, body string) *HTTPResponse {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte(body)))
	if err != nil {
		c.Logger.Error("failed to create POST request", "error", err, "url", url)
		return &HTTPResponse{Error: err.Error()}
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if _, ok := headers["Content-Type"]; !ok {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		c.Logger.Error("failed to perform POST request", "error", err, "url", url)
		return &HTTPResponse{Error: err.Error()}
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		c.Logger.Error("failed to read response body", "error", err, "url", url)
		return &HTTPResponse{StatusCode: resp.StatusCode, Error: err.Error()}
	}

	respHeaders := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			respHeaders[k] = v[0]
		}
	}

	return &HTTPResponse{
		StatusCode: resp.StatusCode,
		Body:       string(bodyBytes),
		Headers:    respHeaders,
	}
}
