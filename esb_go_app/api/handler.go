package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"esb-go-app/i18n"
	"esb-go-app/rabbitmq"
	"esb-go-app/scripting"
	"esb-go-app/storage"
)

type Handler struct {
	Store            *storage.Store
	RabbitMQ         *rabbitmq.RabbitMQ
	Logger           *slog.Logger
	scriptingService *scripting.Service
	I18n             *i18n.Service
}

func NewHandler(s *storage.Store, r *rabbitmq.RabbitMQ, l *slog.Logger, ss *scripting.Service, i18n *i18n.Service) *Handler {
	return &Handler{
		Store:            s,
		RabbitMQ:         r,
		Logger:           l,
		scriptingService: ss,
		I18n:             i18n,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Logger.Info("api handler invoked", "method", r.Method, "path", r.URL.Path)

	switch {
	case strings.HasPrefix(r.URL.Path, "/auth/oidc/token"):
		h.handleGetToken(w, r)
	case strings.HasSuffix(r.URL.Path, "/sys/esb/metadata/channels"):
		h.handleGetMetadataChannels(w, r)
	case strings.HasSuffix(r.URL.Path, "/sys/esb/runtime/channels"):
		h.handleGetRuntimeChannels(w, r)
	default:
		h.Logger.Warn("api path not found", "path", r.URL.Path)
		http.NotFound(w, r)
	}
}

// handleGetToken
func (h *Handler) handleGetToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.Logger.Warn("invalid method for get token", "method", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	reqClientID, reqClientSecret, ok := r.BasicAuth()
	if !ok {
		h.Logger.Warn("basic auth header missing or invalid")
		http.Error(w, "Authorization required", http.StatusUnauthorized)
		return
	}

	app, err := h.Store.GetApplicationByID(reqClientID)
	if err != nil {
		h.Logger.Error("failed to get application by name", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if app == nil {
		h.Logger.Warn("application not found", "client_id", reqClientID)
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	if app.ClientSecret != reqClientSecret {
		h.Logger.Warn("invalid client credentials", "provided_client_id", reqClientID)
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	resp := map[string]string{
		"id_token":     app.IDToken,
		"token_type":   "Bearer",
		"access_token": "Not implemented",
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(resp)
	h.Logger.Info("token issued successfully", "client_id", reqClientID)
}

// handleGetMetadataChannels
func (h *Handler) handleGetMetadataChannels(w http.ResponseWriter, r *http.Request) {
	app, err := h.getAppFromRequest(r)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if app == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	channels, err := h.Store.GetChannelsByAppID(app.ID)
	if err != nil {
		h.Logger.Error("failed to get metadata channels", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	type MetadataChannel struct {
		Process            string `json:"process"`
		ProcessDescription string `json:"processDescription"`
		Channel            string `json:"channel"`
		ChannelDescription string `json:"channelDescription"`
		Access             string `json:"access"`
	}

	result := make([]MetadataChannel, 0, len(channels))
	for _, ch := range channels {
		access := "WRITE_ONLY"
		if ch.Direction == "inbound" {
			access = "READ_ONLY"
		}
		result = append(result, MetadataChannel{
			Process:            h.I18n.Sprintf(r.Header.Get("Accept-Language"), "main"), 
			ProcessDescription: h.I18n.Sprintf(r.Header.Get("Accept-Language"), "Main process"),
			Channel:            ch.Name,
			ChannelDescription: ch.Direction,
			Access:             access,
		})
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(result)
	h.Logger.Info("metadata channels served", "app_id", app.ID)
}

// handleGetRuntimeChannels
func (h *Handler) handleGetRuntimeChannels(w http.ResponseWriter, r *http.Request) {
	app, err := h.getAppFromRequest(r)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if app == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	channels, err := h.Store.GetChannelsByAppID(app.ID)
	if err != nil {
		h.Logger.Error("failed to get runtime channels", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	type RuntimeChannel struct {
		Process     string `json:"process"`
		Channel     string `json:"channel"`
		Destination string `json:"destination"`
	}

	items := make([]RuntimeChannel, 0, len(channels))
	for _, ch := range channels {
		items = append(items, RuntimeChannel{
			Process:     "main",
			Channel:     ch.Name,
			Destination: ch.Destination,
		})
	}

	responseBody := map[string]interface{}{
		"items": items,
		"port":  5672,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(responseBody)
	h.Logger.Info("runtime channels served", "app_id", app.ID)
}

// getAppFromRequest
func (h *Handler) getAppFromRequest(r *http.Request) (*storage.Application, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		h.Logger.Warn("authorization header missing")
		return nil, nil
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 {
		h.Logger.Warn("invalid authorization header format")
		return nil, nil
	}

	authScheme := strings.ToLower(parts[0])

	switch authScheme {
	case "bearer":
		token := parts[1]
		app, err := h.Store.GetApplicationByIDToken(token)
		if err != nil {
			h.Logger.Error("failed to get application by token", "error", err)
			return nil, err
		}
		if app == nil {
			h.Logger.Warn("app not found for token")
			return nil, nil
		}
		return app, nil

	case "basic":
		reqClientID, reqClientSecret, ok := r.BasicAuth()
		if !ok {
			h.Logger.Warn("basic auth header missing or invalid")
			return nil, nil
		}

		app, err := h.Store.GetApplicationByID(reqClientID)
		if err != nil {
			h.Logger.Error("failed to get application by name", "error", err, "client_id", reqClientID)
			return nil, err
		}
		if app == nil {
			h.Logger.Warn("application not found for basic auth", "client_id", reqClientID)
			return nil, nil
		}

		if app.ClientSecret != reqClientSecret {
			h.Logger.Warn("invalid client secret for basic auth", "client_id", reqClientID)
			return nil, nil
		}
		h.Logger.Info("successfully authenticated via basic auth", "client_id", reqClientID)
		return app, nil

	default:
		h.Logger.Warn("unsupported authorization scheme", "scheme", authScheme)
		return nil, nil
	}
}
