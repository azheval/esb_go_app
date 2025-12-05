package admin

import (
	"esb-go-app/rabbitmq"
	"esb-go-app/storage"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type PageData struct {
	Applications        []storage.Application
	Application         *storage.Application
	Channels            []storage.Channel
	StatusMessage       string
	ErrorMessage        string
	TestMessageReceived string
	TestMessageStatus   string
	Routes              []storage.RouteInfo
	OutboundChannels    []storage.ChannelInfo
	InboundChannels     []storage.ChannelInfo
}

type Handler struct {
	Store     *storage.Store
	RabbitMQ  *rabbitmq.RabbitMQ
	Logger    *slog.Logger
	templates map[string]*template.Template
}

func NewHandler(s *storage.Store, r *rabbitmq.RabbitMQ, l *slog.Logger) *Handler {
	templates := make(map[string]*template.Template)
	templates["admin.html"] = template.Must(template.ParseFiles("templates/admin.html", "templates/layout.html"))
	templates["app_details.html"] = template.Must(template.ParseFiles("templates/app_details.html", "templates/layout.html"))
	templates["routes.html"] = template.Must(template.ParseFiles("templates/routes.html", "templates/layout.html"))

	return &Handler{
		Store:     s,
		RabbitMQ:  r,
		Logger:    l,
		templates: templates,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Logger.Info("admin handler invoked", "method", r.Method, "path", r.URL.Path)

	path := strings.TrimPrefix(r.URL.Path, "/admin")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// GET /admin
	if len(parts) == 1 && parts[0] == "" {
		if r.Method == http.MethodGet {
			h.handleListApps(w, r)
			return
		}
	}

	// /admin/routes/*
	if len(parts) > 0 && parts[0] == "routes" {
		if len(parts) == 1 && r.Method == http.MethodGet {
			h.handleRoutes(w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "create" && r.Method == http.MethodPost {
			h.handleCreateRoute(w, r)
			return
		}
		if len(parts) == 3 && parts[1] == "delete" && r.Method == http.MethodPost {
			h.handleDeleteRoute(w, r, parts[2])
			return
		}
	}

	// /admin/queues/*
	if len(parts) > 1 && parts[0] == "queues" {
		if len(parts) == 2 && parts[1] == "recreate" && r.Method == http.MethodPost {
			h.handleRecreateQueues(w, r)
			return
		}
	}

	// /admin/maintenance/*
	if len(parts) == 1 && parts[0] == "maintenance" {
		if r.Method == http.MethodPost {
			h.handleMaintenance(w, r)
			return
		}
	}

	// /admin/app/*
	if len(parts) > 1 && parts[0] == "app" {
		if len(parts) == 2 && parts[1] == "create" && r.Method == http.MethodPost {
			h.handleCreateApp(w, r)
			return
		}

		if len(parts) >= 2 {
			appID := parts[1]

			if len(parts) == 2 && r.Method == http.MethodGet {
				h.handleShowApp(w, r, appID)
				return
			}

			if len(parts) == 3 && parts[2] == "delete" && r.Method == http.MethodPost {
				h.handleDeleteApp(w, r, appID)
				return
			}

			if len(parts) > 3 && parts[2] == "channel" {
				if len(parts) == 4 && parts[3] == "create" && r.Method == http.MethodPost {
					h.handleCreateChannel(w, r, appID)
					return
				}
				if len(parts) == 5 && parts[4] == "delete" && r.Method == http.MethodPost {
					channelID := parts[3]
					h.handleDeleteChannel(w, r, appID, channelID)
					return
				}
				if len(parts) == 5 && parts[4] == "test" {
					h.handleTestExchange(w, r, appID, parts[3])
					return
				}
			}
		}
	}

	http.NotFound(w, r)
}
func (h *Handler) handleListApps(w http.ResponseWriter, r *http.Request) {
	apps, err := h.Store.GetAllApplications()
	if err != nil {
		h.renderError(w, "admin.html", fmt.Sprintf("Failed to retrieve applications: %v", err), http.StatusInternalServerError)
		return
	}

	data := PageData{Applications: apps}
	status := r.URL.Query().Get("status")
	if status == "created" {
		data.StatusMessage = "Приложение успешно создано!"
	} else if status == "recreated" {
		data.StatusMessage = "Все очереди и мосты успешно пересозданы!"
	} else if pruned := r.URL.Query().Get("pruned"); pruned != "" {
		data.StatusMessage = fmt.Sprintf("Удалено 'осиротевших' каналов: %s", pruned)
	}

	h.renderTemplate(w, "admin.html", data)
}

func (h *Handler) handleShowApp(w http.ResponseWriter, r *http.Request, appID string) {
	app, err := h.Store.GetApplicationByID(appID)
	if err != nil || app == nil {
		h.Logger.Error("failed to get application", "error", err, "app_id", appID)
		http.NotFound(w, r)
		return
	}

	channels, err := h.Store.GetChannelsByAppID(appID)
	if err != nil {
		h.renderError(w, "app_details.html", fmt.Sprintf("Failed to retrieve channels: %v", err), http.StatusInternalServerError)
		return
	}

	data := PageData{Application: app, Channels: channels}

	status := r.URL.Query().Get("status")
	if status == "sent" {
		data.TestMessageStatus = "Тестовое сообщение успешно отправлено в постоянный обменник."
	}

	errMsg := r.URL.Query().Get("error")
	if errMsg == "send_failed" {
		data.ErrorMessage = "Не удалось отправить тестовое сообщение."
	} else if errMsg == "receive_failed" {
		data.ErrorMessage = "Не удалось получить тестовое сообщение."
	}

	h.renderTemplate(w, "app_details.html", data)
}

func (h *Handler) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderError(w, "admin.html", "Failed to parse form.", http.StatusBadRequest)
		return
	}
	appName := r.FormValue("name")
	if appName == "" {
		h.renderError(w, "admin.html", "Application name cannot be empty.", http.StatusBadRequest)
		return
	}

	app := &storage.Application{
		ID:           uuid.New().String(),
		Name:         appName,
		ClientSecret: uuid.New().String(),
		IDToken:      uuid.New().String(),
	}

	if err := h.Store.CreateApplication(app); err != nil {
		h.renderError(w, "admin.html", fmt.Sprintf("Failed to create application: %v", err), http.StatusInternalServerError)
		return
	}

	h.Logger.Info("application created successfully", "app_name", app.Name, "app_id", app.ID)
	http.Redirect(w, r, "/admin?status=created", http.StatusSeeOther)
}

func (h *Handler) handleDeleteApp(w http.ResponseWriter, r *http.Request, appID string) {
	if err := h.Store.DeleteApplication(appID); err != nil {
		h.renderError(w, "admin.html", fmt.Sprintf("Failed to delete application: %v", err), http.StatusInternalServerError)
		return
	}
	h.Logger.Info("application deleted successfully", "app_id", appID)
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (h *Handler) handleCreateChannel(w http.ResponseWriter, r *http.Request, appID string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	ch := &storage.Channel{
		ID:            uuid.New().String(),
		ApplicationID: appID,
		Name:          r.FormValue("name"),
		Direction:     r.FormValue("direction"),
		Destination:   r.FormValue("destination"),
	}

	if ch.Name == "" || ch.Destination == "" {
		http.Error(w, "Channel name and destination are required.", http.StatusBadRequest)
		return
	}

	if err := h.RabbitMQ.SetupDurableTopology(ch.Destination); err != nil {
		h.Logger.Error("failed to setup durable rabbitmq topology", "error", err)
		http.Error(w, "Failed to setup RabbitMQ topology.", http.StatusInternalServerError)
		return
	}

	if err := h.Store.CreateChannel(ch); err != nil {
		h.Logger.Error("failed to save channel to db", "error", err)
		http.Error(w, "Failed to save channel.", http.StatusInternalServerError)
		return
	}

	// Запускаем соответствующий воркер в зависимости от направления
	if ch.Direction == "inbound" {
		h.RabbitMQ.StartInboundForwarder(ch.Destination)
	} else if ch.Direction == "outbound" {
		h.RabbitMQ.StartOutboundCollector(ch.Destination)
	}

	h.Logger.Info("channel created successfully", "channel_name", ch.Name, "app_id", appID)
	http.Redirect(w, r, fmt.Sprintf("/admin/app/%s", appID), http.StatusSeeOther)
}

func (h *Handler) handleTestExchange(w http.ResponseWriter, r *http.Request, appID string, channelID string) {
	channel, err := h.Store.GetChannelByID(channelID)
	if err != nil || channel == nil {
		h.Logger.Error("failed to get channel for test exchange", "error", err, "channel_id", channelID)
		http.NotFound(w, r)
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}

		// Определяем действие: отправить или получить
		action := r.FormValue("action")

		if action == "receive" {
			// --- Логика получения сообщения ---
			queueName := "durable_queue_for_" + channel.Destination
			body, ok, err := h.RabbitMQ.GetOneMessage(queueName)
			if err != nil {
				h.Logger.Error("failed to get test message", "error", err)
				http.Redirect(w, r, fmt.Sprintf("/admin/app/%s?error=receive_failed", appID), http.StatusSeeOther)
				return
			}

			app, _ := h.Store.GetApplicationByID(appID)
			channels, _ := h.Store.GetChannelsByAppID(appID)
			data := PageData{Application: app, Channels: channels}
			if ok {
				data.TestMessageReceived = body
				data.TestMessageStatus = "1 сообщение получено и удалено из постоянной очереди."
			} else {
				data.TestMessageStatus = "Постоянная очередь-хранилище пуста."
			}
			h.renderTemplate(w, "app_details.html", data)
			return

		} else {
			// --- Логика отправки сообщения (по умолчанию) ---
			payload := r.FormValue("payload")
			if payload == "" {
				payload = fmt.Sprintf(`{"test_message": "hello from admin", "timestamp": "%s"}`, time.Now().Format(time.RFC3339))
			}

			exchangeName := "durable_exchange_for_" + channel.Destination
			err := h.RabbitMQ.Publish(exchangeName, "", payload)
			if err != nil {
				h.Logger.Error("failed to publish test message", "error", err)
				http.Redirect(w, r, fmt.Sprintf("/admin/app/%s?error=send_failed", appID), http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, fmt.Sprintf("/admin/app/%s?status=sent", appID), http.StatusSeeOther)
			return
		}
	}

	// Если GET-запрос, просто показываем страницу
	h.handleShowApp(w, r, appID)
}

func (h *Handler) handleDeleteChannel(w http.ResponseWriter, r *http.Request, appID, channelID string) {
	if err := h.Store.DeleteChannel(channelID); err != nil {
		h.Logger.Error("failed to delete channel", "error", err)
	}

	h.Logger.Info("channel deleted successfully", "channel_id", channelID)
	http.Redirect(w, r, fmt.Sprintf("/admin/app/%s", appID), http.StatusSeeOther)
}

func (h *Handler) handleMaintenance(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderError(w, "admin.html", "Failed to parse form.", http.StatusBadRequest)
		return
	}

	action := r.FormValue("action")
	if action == "prune_orphaned_channels" {
		count, err := h.Store.DeleteOrphanedChannels()
		if err != nil {
			h.renderError(w, "admin.html", fmt.Sprintf("Failed to prune orphaned channels: %v", err), http.StatusInternalServerError)
			return
		}
		h.Logger.Info("pruned orphaned channels", "count", count)
		http.Redirect(w, r, fmt.Sprintf("/admin?pruned=%d", count), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (h *Handler) handleRecreateQueues(w http.ResponseWriter, r *http.Request) {
	apps, err := h.Store.GetAllApplications()
	if err != nil {
		h.renderError(w, "admin.html", fmt.Sprintf("Failed to retrieve applications: %v", err), http.StatusInternalServerError)
		return
	}

	for _, app := range apps {
		channels, err := h.Store.GetChannelsByAppID(app.ID)
		if err != nil {
			h.renderError(w, "admin.html", fmt.Sprintf("Failed to retrieve channels for app %s: %v", app.Name, err), http.StatusInternalServerError)
			return
		}

		for _, ch := range channels {
			if err := h.RabbitMQ.SetupDurableTopology(ch.Destination); err != nil {
				h.renderError(w, "admin.html", fmt.Sprintf("Failed to setup durable topology for channel %s: %v", ch.Name, err), http.StatusInternalServerError)
				return
			}

			if ch.Direction == "inbound" {
				h.RabbitMQ.StartInboundForwarder(ch.Destination)
			} else if ch.Direction == "outbound" {
				h.RabbitMQ.StartOutboundCollector(ch.Destination)
			}

			h.Logger.Info("durable topology and worker started", "channel_name", ch.Name, "direction", ch.Direction)
		}
	}

	h.Logger.Info("all queues recreated successfully")
	http.Redirect(w, r, "/admin?status=recreated", http.StatusSeeOther)
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

// --- Route Handlers ---

func (h *Handler) handleRoutes(w http.ResponseWriter, r *http.Request) {
	routes, err := h.Store.GetAllRoutes()
	if err != nil {
		h.renderError(w, "routes.html", "Failed to retrieve routes: "+err.Error(), http.StatusInternalServerError)
		return
	}

	outbound, err := h.Store.GetAllRoutableChannels("outbound")
	if err != nil {
		h.renderError(w, "routes.html", "Failed to retrieve outbound channels: "+err.Error(), http.StatusInternalServerError)
		return
	}

	inbound, err := h.Store.GetAllRoutableChannels("inbound")
	if err != nil {
		h.renderError(w, "routes.html", "Failed to retrieve inbound channels: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := PageData{
		Routes:           routes,
		OutboundChannels: outbound,
		InboundChannels:  inbound,
	}
	status := r.URL.Query().Get("status")
	if status == "created" {
		data.StatusMessage = "Маршрут успешно создан!"
	} else if status == "deleted" {
		data.StatusMessage = "Маршрут удален."
	}

	h.renderTemplate(w, "routes.html", data)
}

func (h *Handler) handleCreateRoute(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderError(w, "routes.html", "Failed to parse form.", http.StatusBadRequest)
		return
	}
	sourceChannelID := r.FormValue("source_channel_id")
	destChannelID := r.FormValue("destination_channel_id")

	if sourceChannelID == "" || destChannelID == "" {
		h.renderError(w, "routes.html", "Source and destination channels must be selected.", http.StatusBadRequest)
		return
	}

	route := &storage.Route{
		ID:                   uuid.New().String(),
		SourceChannelID:      sourceChannelID,
		DestinationChannelID: destChannelID,
	}

	if err := h.Store.CreateRoute(route); err != nil {
		h.renderError(w, "routes.html", "Failed to create route: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Запускаем воркер для нового маршрута "на лету"
	sourceChannel, err1 := h.Store.GetChannelByID(sourceChannelID)
	destChannel, err2 := h.Store.GetChannelByID(destChannelID)
	if err1 != nil || err2 != nil {
		h.Logger.Error("failed to get channels for new route worker", "err1", err1, "err2", err2)
	} else {
		h.RabbitMQ.StartRouter(sourceChannel.Destination, destChannel.Destination)
	}

	http.Redirect(w, r, "/admin/routes?status=created", http.StatusSeeOther)
}

func (h *Handler) handleDeleteRoute(w http.ResponseWriter, r *http.Request, routeID string) {
	if err := h.Store.DeleteRoute(routeID); err != nil {
		h.renderError(w, "routes.html", "Failed to delete route: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/routes?status=deleted", http.StatusSeeOther)
}
