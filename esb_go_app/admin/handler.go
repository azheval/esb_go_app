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

	"esb-go-app/i18n"
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
	AcceptLanguage string
	Settings       map[string]string // To hold current settings
}

type Handler struct {
	Store            *storage.Store
	RabbitMQ         *rabbitmq.RabbitMQ
	Logger           *slog.Logger
	templates        map[string]*template.Template
	scriptingService *scripting.Service
	Version          string
	I18n             *i18n.Service
}

func NewHandler(s *storage.Store, r *rabbitmq.RabbitMQ, l *slog.Logger, ss *scripting.Service, version string, i18nService *i18n.Service) *Handler {
	// Add a template function map
	funcMap := template.FuncMap{
		"T": func(key string, args ...interface{}) string {
			// This is a placeholder that will be overridden by the real function in renderTemplate.
			// It's needed so parsing doesn't fail.
			return key
		},
		"substr": func(s string, start, length int) string {
			if start < 0 || start > len(s) {
				return ""
			}
			if start+length > len(s) {
				return s[start:]
			}
			return s[start : start+length]
		},
	}

	templates := make(map[string]*template.Template)
	// Register all templates here and add the function map
	templates["admin.html"] = template.Must(template.New("admin.html").Funcs(funcMap).ParseFiles("templates/admin.html", "templates/layout.html"))
	templates["app_details.html"] = template.Must(template.New("app_details.html").Funcs(funcMap).ParseFiles("templates/app_details.html", "templates/layout.html"))
	templates["channel_details.html"] = template.Must(template.New("channel_details.html").Funcs(funcMap).ParseFiles("templates/channel_details.html", "templates/layout.html"))
	templates["routes.html"] = template.Must(template.New("routes.html").Funcs(funcMap).ParseFiles("templates/routes.html", "templates/layout.html"))
	templates["route_details.html"] = template.Must(template.New("route_details.html").Funcs(funcMap).ParseFiles("templates/route_details.html", "templates/layout.html"))
	templates["transformations.html"] = template.Must(template.New("transformations.html").Funcs(funcMap).ParseFiles("templates/transformations.html", "templates/layout.html"))
	templates["transformation_details.html"] = template.Must(template.New("transformation_details.html").Funcs(funcMap).ParseFiles("templates/transformation_details.html", "templates/layout.html"))
	templates["collectors.html"] = template.Must(template.New("collectors.html").Funcs(funcMap).ParseFiles("templates/collectors.html", "templates/layout.html"))
	templates["collector_details.html"] = template.Must(template.New("collector_details.html").Funcs(funcMap).ParseFiles("templates/collector_details.html", "templates/layout.html"))
	templates["maintenance_queues.html"] = template.Must(template.New("maintenance_queues.html").Funcs(funcMap).ParseFiles("templates/maintenance_queues.html", "templates/layout.html"))
	templates["integrations.html"] = template.Must(template.New("integrations.html").Funcs(funcMap).ParseFiles("templates/integrations.html", "templates/layout.html"))
	templates["integration_details.html"] = template.Must(template.New("integration_details.html").Funcs(funcMap).ParseFiles("templates/integration_details.html", "templates/layout.html"))

	return &Handler{
		Store:            s,
		RabbitMQ:         r,
		Logger:           l,
		templates:        templates,
		scriptingService: ss,
		Version:          version,
		I18n:             i18nService, // Assign i18n service
	}
}

// determineLanguage determines the language for the request.
// It prioritizes the language set in the database, falling back to the Accept-Language header.
func (h *Handler) determineLanguage(r *http.Request) string {
	// 1. Try to get language from DB
	lang, err := h.Store.GetSetting("language")
	if err != nil {
		h.Logger.Error("failed to get language setting from DB", "error", err)
		// Fall through to using header
	}
	if lang != "" {
		return lang
	}

	// 2. Fallback to Accept-Language header
	return r.Header.Get("Accept-Language")
}

// ServeHTTP handles all incoming HTTP requests for the /admin path.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Logger.Info("admin handler invoked", "method", r.Method, "path", r.URL.Path)

	path := strings.TrimPrefix(r.URL.Path, "/admin")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if r.Method == http.MethodPost && path == "/settings/update" {
		h.handleUpdateSettings(w, r)
		return
	}

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

// handleUpdateSettings saves application-wide settings.
func (h *Handler) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderError(w, "admin.html", "Failed to parse form.", http.StatusBadRequest, r)
		return
	}

	lang := r.FormValue("language")
	if lang != "" {
		if err := h.Store.SetSetting("language", lang); err != nil {
			h.renderError(w, "admin.html", "Failed to save language setting.", http.StatusInternalServerError, r)
			return
		}
	}

	http.Redirect(w, r, "/admin?status=settings_updated", http.StatusSeeOther)
}

// This function will be removed once appRoutes correctly handles the root path.
func (h *Handler) handleListAppsLegacy(w http.ResponseWriter, r *http.Request) {
	lang := h.determineLanguage(r)

	apps, err := h.Store.GetAllApplications()
	if err != nil {
		h.renderError(w, "admin.html", fmt.Sprintf("Failed to retrieve applications: %v", err), http.StatusInternalServerError, r)
		return
	}

	// Get language setting for the dropdown
	currentLang, err := h.Store.GetSetting("language")
	if err != nil {
		h.Logger.Error("failed to get language setting for admin page", "error", err)
	}

	data := PageData{
		Applications:   apps,
		Version:        h.Version,
		AcceptLanguage: lang,
		Settings:       map[string]string{"language": currentLang},
	}

	status := r.URL.Query().Get("status")
	if status == "created" {
		data.StatusMessage = h.I18n.Sprintf(lang, "Application created successfully!")
	} else if status == "recreated" {
		data.StatusMessage = h.I18n.Sprintf(lang, "All queues and routes recreated successfully!")
	} else if pruned := r.URL.Query().Get("pruned"); pruned != "" {
		data.StatusMessage = h.I18n.Sprintf(lang, "Pruned orphan channels: %s", pruned)
	} else if status == "settings_updated" {
		data.StatusMessage = h.I18n.Sprintf(lang, "Settings updated successfully.")
	}


	h.renderTemplate(w, "admin.html", data)
}

func (h *Handler) renderTemplate(w http.ResponseWriter, name string, data PageData) {
	tmpl, ok := h.templates[name]
	if !ok {
		http.Error(w, fmt.Sprintf("Template %s not found", name), http.StatusInternalServerError)
		return
	}

	// Clone the template set for each request to add dynamic functions
	clonedTmpl, err := tmpl.Clone()
	if err != nil {
		h.Logger.Error("failed to clone template", "error", err, "template", name)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// The FuncMap in NewHandler provides a placeholder for 'T'.
	// Here we override it with a request-specific implementation.
	clonedTmpl.Funcs(template.FuncMap{
		"T": func(key string, args ...interface{}) string {
			return h.I18n.Sprintf(data.AcceptLanguage, key, args...)
		},
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err = clonedTmpl.ExecuteTemplate(w, "layout", data)
	if err != nil {
		h.Logger.Error("failed to execute template", "error", err, "template", name)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *Handler) renderError(w http.ResponseWriter, templateName string, errorMessage string, statusCode int, r *http.Request) {
	lang := h.determineLanguage(r)
	data := PageData{
		ErrorMessage:   errorMessage,
		AcceptLanguage: lang,
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
