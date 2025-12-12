package admin

import (
	"net/http"

	"github.com/google/uuid"

	"esb-go-app/storage"
)

// CollectorRoutes handles routing for /admin/collectors/* paths.
func CollectorRoutes(h *Handler, w http.ResponseWriter, r *http.Request, parts []string) {
	if r.Method == http.MethodGet {
		if len(parts) == 0 || (len(parts) == 1 && parts[0] == "") {
			h.handleListCollectors(w, r)
			return
		}
		if len(parts) == 1 {
			collectorID := parts[0]
			h.handleViewCollector(w, r, collectorID)
			return
		}
	}

	if r.Method == http.MethodPost {
		if len(parts) == 1 && parts[0] == "create" {
			h.handleCreateCollector(w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "update" {
			collectorID := parts[0]
			h.handleUpdateCollector(w, r, collectorID)
			return
		}
		if len(parts) == 2 && parts[1] == "delete" {
			collectorID := parts[0]
			h.handleDeleteCollector(w, r, collectorID)
			return
		}
	}

	http.NotFound(w, r)
}

func (h *Handler) handleListCollectors(w http.ResponseWriter, r *http.Request) {
	lang := h.determineLanguage(r)
	collectors, err := h.Store.GetAllCollectors()
	if err != nil {
		h.renderError(w, "collectors.html", h.I18n.Sprintf(lang, "Failed to retrieve collectors: %s", err.Error()), http.StatusInternalServerError, r)
		return
	}

	integrations, err := h.Store.GetAllIntegrations()
	if err != nil {
		h.renderError(w, "collectors.html", h.I18n.Sprintf(lang, "Failed to retrieve integrations: %s", err.Error()), http.StatusInternalServerError, r)
		return
	}

	data := PageData{
		Collectors:     collectors,
		Integrations:   integrations,
		AcceptLanguage: lang,
	}

	status := r.URL.Query().Get("status")
	if status == "created" {
		data.StatusMessage = h.I18n.Sprintf(lang, "Collector created successfully!")
	} else if status == "deleted" {
		data.StatusMessage = h.I18n.Sprintf(lang, "Collector deleted.")
	} else if status == "updated" {
		data.StatusMessage = h.I18n.Sprintf(lang, "Collector updated successfully!")
	}

	h.renderTemplate(w, "collectors.html", data)
}

func (h *Handler) handleViewCollector(w http.ResponseWriter, r *http.Request, collectorID string) {
	lang := h.determineLanguage(r)
	collector, err := h.Store.GetCollectorByID(collectorID)
	if err != nil {
		h.renderError(w, "collectors.html", h.I18n.Sprintf(lang, "Failed to retrieve collector: %s", err.Error()), http.StatusInternalServerError, r)
		return
	}
	if collector == nil {
		h.renderError(w, "collectors.html", h.I18n.Sprintf(lang, "Collector not found."), http.StatusNotFound, r)
		return
	}

	integrations, err := h.Store.GetAllIntegrations()
	if err != nil {
		h.renderError(w, "collector_details.html", h.I18n.Sprintf(lang, "Failed to retrieve integrations: %s", err.Error()), http.StatusInternalServerError, r)
		return
	}

	data := PageData{
		Collector:      collector,
		Integrations:   integrations,
		AcceptLanguage: lang,
	}
	if collector.IntegrationID != nil {
		data.SelectedIntegrationID = *collector.IntegrationID
	}

	h.renderTemplate(w, "collector_details.html", data)
}

func (h *Handler) handleCreateCollector(w http.ResponseWriter, r *http.Request) {
	lang := h.determineLanguage(r)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, "collectors.html", h.I18n.Sprintf(lang, "Failed to parse form."), http.StatusBadRequest, r)
		return
	}

	integrationID := r.FormValue("integration_id")
	var integrationIDPtr *string
	if integrationID != "" {
		integrationIDPtr = &integrationID
	}

	collector := &storage.Collector{
		ID:            uuid.New().String(),
		Name:          r.FormValue("name"),
		Schedule:      r.FormValue("schedule"),
		Engine:        r.FormValue("engine"),
		Script:        r.FormValue("script"),
		IntegrationID: integrationIDPtr,
	}

	if collector.Name == "" || collector.Schedule == "" || collector.Engine == "" || collector.Script == "" {
		h.renderError(w, "collectors.html", h.I18n.Sprintf(lang, "All fields except integration are required."), http.StatusBadRequest, r)
		return
	}

	if err := h.Store.CreateCollector(collector); err != nil {
		h.renderError(w, "collectors.html", h.I18n.Sprintf(lang, "Failed to create collector: %s", err.Error()), http.StatusInternalServerError, r)
		return
	}

	h.Logger.Info("collector created successfully", "collector_name", collector.Name, "collector_id", collector.ID)
	http.Redirect(w, r, "/admin/collectors?status=created", http.StatusSeeOther)
}

func (h *Handler) handleUpdateCollector(w http.ResponseWriter, r *http.Request, collectorID string) {
	lang := h.determineLanguage(r)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, "collector_details.html", h.I18n.Sprintf(lang, "Failed to parse form."), http.StatusBadRequest, r)
		return
	}

	integrationID := r.FormValue("integration_id")
	var integrationIDPtr *string
	if integrationID != "" {
		integrationIDPtr = &integrationID
	}

	collector := &storage.Collector{
		ID:            collectorID,
		Name:          r.FormValue("name"),
		Schedule:      r.FormValue("schedule"),
		Engine:        r.FormValue("engine"),
		Script:        r.FormValue("script"),
		IntegrationID: integrationIDPtr,
	}

	if collector.Name == "" || collector.Schedule == "" || collector.Engine == "" || collector.Script == "" {
		h.renderError(w, "collector_details.html", h.I18n.Sprintf(lang, "All fields except integration are required."), http.StatusBadRequest, r)
		return
	}

	if err := h.Store.UpdateCollector(collector); err != nil {
		h.renderError(w, "collector_details.html", h.I18n.Sprintf(lang, "Failed to update collector: %s", err.Error()), http.StatusInternalServerError, r)
		return
	}

	h.Logger.Info("collector updated successfully", "collector_id", collectorID)
	http.Redirect(w, r, "/admin/collectors?status=updated", http.StatusSeeOther)
}

func (h *Handler) handleDeleteCollector(w http.ResponseWriter, r *http.Request, collectorID string) {
	lang := h.determineLanguage(r)
	if err := h.Store.DeleteCollector(collectorID); err != nil {
		h.renderError(w, "collectors.html", h.I18n.Sprintf(lang, "Failed to delete collector: %s", err.Error()), http.StatusInternalServerError, r)
		return
	}

	// TODO: Stop the collector worker if it's running

	h.Logger.Info("collector deleted successfully", "collector_id", collectorID)
	http.Redirect(w, r, "/admin/collectors?status=deleted", http.StatusSeeOther)
}
