package admin

import (
	"fmt"
	"net/http"
	"strings"
)

// MaintenanceRoutes handles routing for /admin/maintenance/* paths.
func MaintenanceRoutes(h *Handler, w http.ResponseWriter, r *http.Request, parts []string) {
	// GET /admin/maintenance/queues
	if r.Method == http.MethodGet && len(parts) == 1 && parts[0] == "queues" {
		h.handleQueueReconciliation(w, r)
		return
	}

	// POST /admin/maintenance
	if r.Method == http.MethodPost && (len(parts) == 0 || (len(parts) == 1 && parts[0] == "")) {
		h.handleMaintenanceActions(w, r)
		return
	}

	http.NotFound(w, r)
}

func (h *Handler) handleMaintenanceActions(w http.ResponseWriter, r *http.Request) {
	lang := h.determineLanguage(r)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, "admin.html", h.I18n.Sprintf(lang, "Failed to parse form."), http.StatusBadRequest, r)
		return
	}

	action := r.FormValue("action")
	switch action {
	case "prune_orphaned_channels":
		h.handlePruneOrphanedChannels(w, r)
	default:
		h.renderError(w, "admin.html", h.I18n.Sprintf(lang, "Unknown maintenance action."), http.StatusBadRequest, r)
	}
}

func (h *Handler) handlePruneOrphanedChannels(w http.ResponseWriter, r *http.Request) {
	lang := h.determineLanguage(r)
	count, err := h.Store.DeleteOrphanedChannels()
	if err != nil {
		h.renderError(w, "admin.html", h.I18n.Sprintf(lang, "Failed to prune orphaned channels: %s", err.Error()), http.StatusInternalServerError, r)
		return
	}
	h.Logger.Info("pruned orphaned channels", "count", count)
	http.Redirect(w, r, fmt.Sprintf("/admin?pruned=%d", count), http.StatusSeeOther)
}

type QueueReconResult struct {
	DBQueues         []string
	RabbitMQQueues   []string
	OrphanedQueues   []string // In RabbitMQ but not in DB
	MissingQueues    []string // In DB but not in RabbitMQ
	MatchingQueues   []string
}

func (h *Handler) handleQueueReconciliation(w http.ResponseWriter, r *http.Request) {
	lang := h.determineLanguage(r)
	// 1. Get all queues from the database (by getting all channels)
	dbChannels, err := h.Store.GetAllChannels()
	if err != nil {
		h.renderError(w, "maintenance_queues.html", h.I18n.Sprintf(lang, "Failed to retrieve channels from database: %s", err.Error()), http.StatusInternalServerError, r)
		return
	}
	dbQueueMap := make(map[string]bool)
	var dbQueueList []string
	for _, ch := range dbChannels {
		// Assuming the convention is "durable_queue_for_" + destination name
		qName := "durable_queue_for_" + ch.Destination
		if !dbQueueMap[qName] {
			dbQueueMap[qName] = true
			dbQueueList = append(dbQueueList, qName)
		}
	}

	// 2. Get all queues from RabbitMQ Management API
	rabbitQueues, err := h.RabbitMQ.ListQueues()
	if err != nil {
		errMsg := h.I18n.Sprintf(lang, "Could not get queue list from RabbitMQ Management API. Ensure the API is accessible and credentials are correct in config.json. Error: %v", err)
		h.renderError(w, "maintenance_queues.html", errMsg, http.StatusInternalServerError, r)
		return
	}

	rabbitQueueMap := make(map[string]bool)
	var rabbitQueueList []string
	for _, q := range rabbitQueues {
		// Only consider durable queues managed by this app
		if q.Durable && strings.HasPrefix(q.Name, "durable_queue_for_") {
			rabbitQueueMap[q.Name] = true
			rabbitQueueList = append(rabbitQueueList, q.Name)
		}
	}

	// 3. Compare the lists
	result := &QueueReconResult{
		DBQueues:       dbQueueList,
		RabbitMQQueues: rabbitQueueList,
	}
	for qName := range rabbitQueueMap {
		if !dbQueueMap[qName] {
			result.OrphanedQueues = append(result.OrphanedQueues, qName)
		} else {
			result.MatchingQueues = append(result.MatchingQueues, qName)
		}
	}
	for qName := range dbQueueMap {
		if !rabbitQueueMap[qName] {
			result.MissingQueues = append(result.MissingQueues, qName)
		}
	}

	// 4. Render the template
	h.renderTemplate(w, "maintenance_queues.html", PageData{
		QueueRecon:     result,
		AcceptLanguage: lang,
	})
}
