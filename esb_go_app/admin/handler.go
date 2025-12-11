package admin

import (
	"esb-go-app/rabbitmq"
	"esb-go-app/scripting"
	"esb-go-app/storage"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
)

type PageData struct {
	Applications          []storage.Application
	Application           *storage.Application
	Channels              []storage.Channel
	Channel               *storage.Channel // For detail pages
	StatusMessage         string
	ErrorMessage          string
	TestMessageReceived   string
	TestMessageStatus     string
	Routes                []storage.RouteInfo
	Route                 *storage.RouteInfo // For detail pages
	RouteSources          []storage.RouteSource
	InboundChannels       []storage.ChannelInfo
	DestinationChannels   []storage.ChannelInfo // Unified list for destinations
	Transformations       []storage.Transformation
	Transformation        *storage.Transformation // For detail pages
	Collectors            []storage.Collector
	Collector             *storage.Collector // For detail pages
	Integrations          []storage.Integration
	Integration           *storage.Integration // For detail pages
	Version               string
	QueueRecon            *QueueReconResult
	SelectedIntegrationID string
	MermaidDiagram        string
}

type Handler struct {
	Store            *storage.Store
	RabbitMQ         *rabbitmq.RabbitMQ
	Logger           *slog.Logger
	templates        map[string]*template.Template
	scriptingService *scripting.Service
	Version          string
}

func NewHandler(s *storage.Store, r *rabbitmq.RabbitMQ, l *slog.Logger, ss *scripting.Service, version string) *Handler {
	templates := make(map[string]*template.Template)
	// Register all templates here
	templates["admin.html"] = template.Must(template.ParseFiles("templates/admin.html", "templates/layout.html"))
	templates["app_details.html"] = template.Must(template.ParseFiles("templates/app_details.html", "templates/layout.html"))
	templates["channel_details.html"] = template.Must(template.ParseFiles("templates/channel_details.html", "templates/layout.html"))
	templates["routes.html"] = template.Must(template.ParseFiles("templates/routes.html", "templates/layout.html"))
	templates["route_details.html"] = template.Must(template.ParseFiles("templates/route_details.html", "templates/layout.html"))
	templates["transformations.html"] = template.Must(template.ParseFiles("templates/transformations.html", "templates/layout.html"))
	templates["transformation_details.html"] = template.Must(template.ParseFiles("templates/transformation_details.html", "templates/layout.html"))
	templates["collectors.html"] = template.Must(template.ParseFiles("templates/collectors.html", "templates/layout.html"))
	templates["collector_details.html"] = template.Must(template.ParseFiles("templates/collector_details.html", "templates/layout.html"))
	templates["maintenance_queues.html"] = template.Must(template.ParseFiles("templates/maintenance_queues.html", "templates/layout.html"))
	templates["integrations.html"] = template.Must(template.ParseFiles("templates/integrations.html", "templates/layout.html"))
	templates["integration_details.html"] = template.Must(template.ParseFiles("templates/integration_details.html", "templates/layout.html"))

	return &Handler{
		Store:            s,
		RabbitMQ:         r,
		Logger:           l,
		templates:        templates,
		scriptingService: ss,
		Version:          version,
	}
}

// ServeHTTP handles all incoming HTTP requests for the /admin path.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Logger.Info("admin handler invoked", "method", r.Method, "path", r.URL.Path)

	path := strings.TrimPrefix(r.URL.Path, "/admin")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if len(parts) == 0 || parts[0] == "" {
		if r.Method == http.MethodGet {
			h.handleListAppsLegacy(w, r)
			return
		}
		http.NotFound(w, r)
		return
	}

	subPath := parts[1:]
	switch parts[0] {
	case "app":
		AppRoutes(h, w, r, subPath)
	case "routes":
		RouteRoutes(h, w, r, subPath)
	case "transformations":
		TransformationRoutes(h, w, r, subPath)
	case "collectors":
		CollectorRoutes(h, w, r, subPath)
	case "integrations":
		IntegrationRoutes(h, w, r, subPath)
	case "maintenance":
		MaintenanceRoutes(h, w, r, subPath)
	default:
		http.NotFound(w, r)
	}
}

// This function will be removed once appRoutes correctly handles the root path.
func (h *Handler) handleListAppsLegacy(w http.ResponseWriter, r *http.Request) {
	apps, err := h.Store.GetAllApplications()
	if err != nil {
		h.renderError(w, "admin.html", fmt.Sprintf("Failed to retrieve applications: %v", err), http.StatusInternalServerError)
		return
	}

	data := PageData{
		Applications: apps,
		Version:      h.Version,
	}
	status := r.URL.Query().Get("status")
	if status == "created" {
		data.StatusMessage = "Приложение успешно создано!"
	} else if status == "recreated" {
		data.StatusMessage = "Все очереди и маршруты успешно пересозданы!"
	} else if pruned := r.URL.Query().Get("pruned"); pruned != "" {
		data.StatusMessage = fmt.Sprintf("Удалено 'осиротевших' каналов: %s", pruned)
	}

	h.renderTemplate(w, "admin.html", data)
}

func (h *Handler) renderTemplate(w http.ResponseWriter, name string, data PageData) {
	tmpl, ok := h.templates[name]
	if !ok {
		http.Error(w, fmt.Sprintf("Template %s not found", name), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err := tmpl.ExecuteTemplate(w, "layout", data)
	if err != nil {
		h.Logger.Error("failed to execute template", "error", err, "template", name)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *Handler) renderError(w http.ResponseWriter, templateName string, errorMessage string, statusCode int) {
	data := PageData{
		ErrorMessage: errorMessage,
	}
	w.WriteHeader(statusCode)
	h.renderTemplate(w, templateName, data)
}

// This function needs to be a method of Handler to access h.Store and h.Logger.
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
