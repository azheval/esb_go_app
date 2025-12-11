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
	collectors, err := h.Store.GetAllCollectors()
	if err != nil {
		h.renderError(w, "collectors.html", "Failed to retrieve collectors: "+err.Error(), http.StatusInternalServerError)
		return
	}

	integrations, err := h.Store.GetAllIntegrations()
	if err != nil {
		h.renderError(w, "collectors.html", "Failed to retrieve integrations: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := PageData{
		Collectors:   collectors,
		Integrations: integrations,
	}

	status := r.URL.Query().Get("status")
	if status == "created" {
		data.StatusMessage = "Сборщик успешно создан!"
	} else if status == "deleted" {
		data.StatusMessage = "Сборщик удален."
	} else if status == "updated" {
		data.StatusMessage = "Сборщик успешно обновлен."
	}

	h.renderTemplate(w, "collectors.html", data)
}

func (h *Handler) handleViewCollector(w http.ResponseWriter, r *http.Request, collectorID string) {
	collector, err := h.Store.GetCollectorByID(collectorID)
	if err != nil {
		h.renderError(w, "collectors.html", "Failed to retrieve collector: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if collector == nil {
		h.renderError(w, "collectors.html", "Collector not found.", http.StatusNotFound)
		return
	}

	integrations, err := h.Store.GetAllIntegrations()
	if err != nil {
		h.renderError(w, "collector_details.html", "Failed to retrieve integrations: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := PageData{
		Collector:    collector,
		Integrations: integrations,
	}
	if collector.IntegrationID != nil {
		data.SelectedIntegrationID = *collector.IntegrationID
	}

	h.renderTemplate(w, "collector_details.html", data)
}

func (h *Handler) handleCreateCollector(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderError(w, "collectors.html", "Failed to parse form.", http.StatusBadRequest)
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
		h.renderError(w, "collectors.html", "Все поля, кроме интеграции, обязательны для заполнения.", http.StatusBadRequest)
		return
	}

	if err := h.Store.CreateCollector(collector); err != nil {
		h.renderError(w, "collectors.html", "Failed to create collector: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.Logger.Info("collector created successfully", "collector_name", collector.Name, "collector_id", collector.ID)
	http.Redirect(w, r, "/admin/collectors?status=created", http.StatusSeeOther)
}

func (h *Handler) handleUpdateCollector(w http.ResponseWriter, r *http.Request, collectorID string) {
	if err := r.ParseForm(); err != nil {
		h.renderError(w, "collector_details.html", "Failed to parse form.", http.StatusBadRequest)
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
		h.renderError(w, "collector_details.html", "Все поля, кроме интеграции, обязательны для заполнения.", http.StatusBadRequest)
		return
	}

	if err := h.Store.UpdateCollector(collector); err != nil {
		h.renderError(w, "collector_details.html", "Failed to update collector: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.Logger.Info("collector updated successfully", "collector_id", collectorID)
	http.Redirect(w, r, "/admin/collectors?status=updated", http.StatusSeeOther)
}

func (h *Handler) handleDeleteCollector(w http.ResponseWriter, r *http.Request, collectorID string) {
	if err := h.Store.DeleteCollector(collectorID); err != nil {
		h.renderError(w, "collectors.html", "Failed to delete collector: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// TODO: Stop the collector worker if it's running

	h.Logger.Info("collector deleted successfully", "collector_id", collectorID)
	http.Redirect(w, r, "/admin/collectors?status=deleted", http.StatusSeeOther)
}
