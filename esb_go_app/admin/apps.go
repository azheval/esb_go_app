package admin

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"esb-go-app/storage"
)

// AppRoutes handles routing for /admin/app/* paths.
func AppRoutes(h *Handler, w http.ResponseWriter, r *http.Request, parts []string) {
	// POST /admin/app/create
	if r.Method == http.MethodPost && len(parts) == 1 && parts[0] == "create" {
		h.handleCreateApp(w, r)
		return
	}

	// Paths with an appID: /admin/app/{id}/*
	if len(parts) >= 1 {
		appID := parts[0]

		// GET /admin/app/{id}
		if r.Method == http.MethodGet && len(parts) == 1 {
			h.handleShowApp(w, r, appID)
			return
		}

		// POST /admin/app/{id}/delete
		if r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "delete" {
			h.handleDeleteApp(w, r, appID)
			return
		}

		// POST /admin/app/{id}/update
		if r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "update" {
			h.handleUpdateApp(w, r, appID)
			return
		}

		// Nested Channel routes: /admin/app/{id}/channel/*
		if len(parts) > 2 && parts[1] == "channel" {
			ChannelRoutes(h, w, r, appID, parts[2:]) // Pass remaining parts for channel routing
			return
		}
	}

	http.NotFound(w, r)
}

// handleShowApp displays details for a specific application.
func (h *Handler) handleShowApp(w http.ResponseWriter, r *http.Request, appID string) {
	lang := h.determineLanguage(r)
	app, err := h.Store.GetApplicationByID(appID)
	if err != nil || app == nil {
		h.Logger.Error("failed to get application", "error", err, "app_id", appID)
		http.NotFound(w, r)
		return
	}

	channels, err := h.Store.GetChannelsByAppID(appID)
	if err != nil {
		h.renderError(w, "app_details.html", fmt.Sprintf("Failed to retrieve channels: %v", err), http.StatusInternalServerError, r)
		return
	}

	data := PageData{Application: app, Channels: channels, AcceptLanguage: lang}

	status := r.URL.Query().Get("status")
	if status == "channel_created" {
		data.StatusMessage = h.I18n.Sprintf(lang, "Channel created successfully.")
	} else if status == "channel_deleted" {
		data.StatusMessage = h.I18n.Sprintf(lang, "Channel deleted.")
	}

	h.renderTemplate(w, "app_details.html", data)
}

// handleCreateApp creates a new application.
func (h *Handler) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	lang := h.determineLanguage(r)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, "admin.html", "Failed to parse form.", http.StatusBadRequest, r)
		return
	}
	appName := r.FormValue("name")
	if appName == "" {
		h.renderError(w, "admin.html", h.I18n.Sprintf(lang, "Application name cannot be empty."), http.StatusBadRequest, r)
		return
	}

	app := &storage.Application{
		ID:           uuid.New().String(),
		Name:         appName,
		ClientSecret: uuid.New().String(),
		IDToken:      uuid.New().String(),
	}

	if err := h.Store.CreateApplication(app); err != nil {
		h.renderError(w, "admin.html", fmt.Sprintf("Failed to create application: %v", err), http.StatusInternalServerError, r)
		return
	}

	h.Logger.Info("application created successfully", "app_name", app.Name, "app_id", app.ID)
	http.Redirect(w, r, "/admin?status=created", http.StatusSeeOther)
}

// handleUpdateApp updates an application's details.
func (h *Handler) handleUpdateApp(w http.ResponseWriter, r *http.Request, appID string) {
	lang := h.determineLanguage(r)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, "app_details.html", "Failed to parse form.", http.StatusBadRequest, r)
		return
	}
	appName := r.FormValue("name")
	if appName == "" {
		h.renderError(w, "app_details.html", h.I18n.Sprintf(lang, "Application name cannot be empty."), http.StatusBadRequest, r)
		return
	}

	app := &storage.Application{
		ID:   appID,
		Name: appName,
	}

	if err := h.Store.UpdateApplication(app); err != nil {
		h.renderError(w, "app_details.html", fmt.Sprintf("Failed to update application: %v", err), http.StatusInternalServerError, r)
		return
	}

	h.Logger.Info("application updated successfully", "app_id", appID)
	http.Redirect(w, r, fmt.Sprintf("/admin/app/%s?status=updated", appID), http.StatusSeeOther)
}

// handleDeleteApp deletes an application.
func (h *Handler) handleDeleteApp(w http.ResponseWriter, r *http.Request, appID string) {
	if err := h.Store.DeleteApplication(appID); err != nil {
		h.renderError(w, "admin.html", fmt.Sprintf("Failed to delete application: %v", err), http.StatusInternalServerError, r)
		return
	}
	h.Logger.Info("application deleted successfully", "app_id", appID)
	http.Redirect(w, r, "/admin?status=deleted", http.StatusSeeOther)
}
