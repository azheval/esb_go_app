package admin

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"esb-go-app/storage"
)

// ChannelRoutes handles routing for /admin/app/{appID}/channel/* paths.
func ChannelRoutes(h *Handler, w http.ResponseWriter, r *http.Request, appID string, parts []string) {
	// POST /admin/app/{appID}/channel/create
	if r.Method == http.MethodPost && len(parts) == 1 && parts[0] == "create" {
		h.handleCreateChannel(w, r, appID)
		return
	}

	// GET /admin/app/{appID}/channel/{channelID}
	if r.Method == http.MethodGet && len(parts) == 1 {
		channelID := parts[0]
		h.handleViewChannel(w, r, channelID)
		return
	}

	// POST /admin/app/{appID}/channel/{channelID}/update
	if r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "update" {
		channelID := parts[0]
		h.handleUpdateChannel(w, r, appID, channelID)
		return
	}

	// POST /admin/app/{appID}/channel/{channelID}/delete
	if r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "delete" {
		channelID := parts[0]
		h.handleDeleteChannel(w, r, appID, channelID)
		return
	}

	// POST /admin/app/{appID}/channel/{channelID}/test
	if r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "test" {
		channelID := parts[0]
		h.handleTestExchange(w, r, appID, channelID)
		return
	}

	http.NotFound(w, r)
}

func (h *Handler) handleViewChannel(w http.ResponseWriter, r *http.Request, channelID string) {
	channel, err := h.Store.GetChannelByID(channelID)
	if err != nil {
		h.renderError(w, "app_details.html", "Failed to retrieve channel: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if channel == nil {
		h.renderError(w, "app_details.html", "Channel not found.", http.StatusNotFound)
		return
	}

	data := PageData{
		Channel: channel,
	}

	status := r.URL.Query().Get("status")
	if status == "updated" {
		data.StatusMessage = "Канал успешно обновлен!"
	}

	h.renderTemplate(w, "channel_details.html", data)
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
		FanoutMode:    r.FormValue("fanout_mode") == "on",
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

	if ch.Direction == "inbound" {
		h.RabbitMQ.StartInboundForwarder(ch.Destination)
	} else if ch.Direction == "outbound" {
		h.RabbitMQ.StartOutboundCollector(ch.Destination)
	}

	h.Logger.Info("channel created successfully", "channel_name", ch.Name, "app_id", appID)
	http.Redirect(w, r, fmt.Sprintf("/admin/app/%s?status=channel_created", appID), http.StatusSeeOther)
}

func (h *Handler) handleUpdateChannel(w http.ResponseWriter, r *http.Request, appID, channelID string) {
	if err := r.ParseForm(); err != nil {
		h.renderError(w, "channel_details.html", "Failed to parse form.", http.StatusBadRequest)
		return
	}

	// Fetch the existing channel to update its properties
	ch, err := h.Store.GetChannelByID(channelID)
	if err != nil || ch == nil {
		h.renderError(w, "channel_details.html", "Channel not found to update.", http.StatusNotFound)
		return
	}

	// Update properties from form
	ch.Name = r.FormValue("name")
	ch.Direction = r.FormValue("direction")
	ch.Destination = r.FormValue("destination")
	ch.FanoutMode = r.FormValue("fanout_mode") == "on"

	if ch.Name == "" || ch.Destination == "" {
		h.renderError(w, "channel_details.html", "Channel name and destination are required.", http.StatusBadRequest)
		return
	}

	if err := h.Store.UpdateChannel(ch); err != nil {
		h.renderError(w, "channel_details.html", "Failed to update channel: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.Logger.Info("channel updated successfully", "channel_id", channelID)
	http.Redirect(w, r, fmt.Sprintf("/admin/app/%s/channel/%s?status=updated", appID, channelID), http.StatusSeeOther)
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

		action := r.FormValue("action")

		if action == "receive" {
			queueName := "durable_queue_for_" + channel.Destination
			body, ok, err := h.RabbitMQ.GetOneMessage(queueName)
			if err != nil {
				h.Logger.Error("failed to get test message", "error", err)
				http.Redirect(w, r, fmt.Sprintf("/admin/app/%s?error=receive_failed", appID), http.StatusSeeOther)
				return
			}

			app, err := h.Store.GetApplicationByID(appID)
			if err != nil || app == nil {
				h.renderError(w, "app_details.html", "Failed to retrieve application for test.", http.StatusInternalServerError)
				return
			}
			channels, err := h.Store.GetChannelsByAppID(appID)
			if err != nil {
				h.renderError(w, "app_details.html", "Failed to retrieve channels for test.", http.StatusInternalServerError)
				return
			}
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

	h.handleShowApp(w, r, appID) // Render app details page with test form
}

func (h *Handler) handleDeleteChannel(w http.ResponseWriter, r *http.Request, appID, channelID string) {
	if err := h.Store.DeleteChannel(channelID); err != nil {
		h.Logger.Error("failed to delete channel", "error", err)
	}

	h.Logger.Info("channel deleted successfully", "channel_id", channelID)
	http.Redirect(w, r, fmt.Sprintf("/admin/app/%s?status=channel_deleted", appID), http.StatusSeeOther)
}
