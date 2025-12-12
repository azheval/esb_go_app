package admin

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"text/template"

	"github.com/google/uuid"

	"esb-go-app/storage"
)

// IntegrationRoutes handles routing for /admin/integrations/* paths.
func IntegrationRoutes(h *Handler, w http.ResponseWriter, r *http.Request, parts []string) {
	if r.Method == http.MethodGet {
		if len(parts) == 0 || (len(parts) == 1 && parts[0] == "") {
			h.handleListIntegrations(w, r)
			return
		}
		if len(parts) == 1 {
			integrationID := parts[0]
			h.handleViewIntegration(w, r, integrationID)
			return
		}
	}

	if r.Method == http.MethodPost {
		if len(parts) == 1 && parts[0] == "create" {
			h.handleCreateIntegration(w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "delete" {
			integrationID := parts[0]
			h.handleDeleteIntegration(w, r, integrationID)
			return
		}
	}

	http.NotFound(w, r)
}

func (h *Handler) handleListIntegrations(w http.ResponseWriter, r *http.Request) {
	lang := h.determineLanguage(r)
	integrations, err := h.Store.GetAllIntegrations()
	if err != nil {
		h.renderError(w, "integrations.html", h.I18n.Sprintf(lang, "Failed to retrieve integrations: %s", err.Error()), http.StatusInternalServerError, r)
		return
	}

	data := PageData{
		Integrations:   integrations,
		AcceptLanguage: lang,
	}

	status := r.URL.Query().Get("status")
	if status == "created" {
		data.StatusMessage = h.I18n.Sprintf(lang, "Integration created successfully!")
	} else if status == "deleted" {
		data.StatusMessage = h.I18n.Sprintf(lang, "Integration deleted.")
	}

	h.renderTemplate(w, "integrations.html", data)
}

func (h *Handler) handleViewIntegration(w http.ResponseWriter, r *http.Request, integrationID string) {
	lang := h.determineLanguage(r)
	integration, err := h.Store.GetIntegrationByID(integrationID)
	if err != nil || integration == nil {
		h.renderError(w, "integrations.html", h.I18n.Sprintf(lang, "Integration not found: %s", err.Error()), http.StatusNotFound, r)
		return
	}

	collectors, err := h.Store.GetCollectorsByIntegrationID(integrationID)
	if err != nil {
		h.renderError(w, "integration_details.html", h.I18n.Sprintf(lang, "Failed to retrieve collectors: %s", err.Error()), http.StatusInternalServerError, r)
		return
	}

	routes, err := h.Store.GetRoutesByIntegrationID(integrationID)
	if err != nil {
		h.renderError(w, "integration_details.html", h.I18n.Sprintf(lang, "Failed to retrieve routes: %s", err.Error()), http.StatusInternalServerError, r)
		return
	}

	diagram := generateMermaidDiagram(collectors, routes, h.I18n.Sprintf(lang, "(Collector)<br>%s"))

	data := PageData{
		Integration:    integration,
		Collectors:     collectors,
		Routes:         routes,
		MermaidDiagram: diagram,
		AcceptLanguage: lang,
	}

	h.renderTemplate(w, "integration_details.html", data)
}

func (h *Handler) handleCreateIntegration(w http.ResponseWriter, r *http.Request) {
	lang := h.determineLanguage(r)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, "integrations.html", h.I18n.Sprintf(lang, "Failed to parse form."), http.StatusBadRequest, r)
		return
	}

	integration := &storage.Integration{
		ID:          uuid.New().String(),
		Name:        r.FormValue("name"),
		Description: r.FormValue("description"),
	}

	if integration.Name == "" {
		h.renderError(w, "integrations.html", h.I18n.Sprintf(lang, "Integration name cannot be empty."), http.StatusBadRequest, r)
		return
	}

	if err := h.Store.CreateIntegration(integration); err != nil {
		h.renderError(w, "integrations.html", h.I18n.Sprintf(lang, "Failed to create integration: %s", err.Error()), http.StatusInternalServerError, r)
		return
	}

	h.Logger.Info("integration created successfully", "integration_name", integration.Name, "integration_id", integration.ID)
	http.Redirect(w, r, "/admin/integrations?status=created", http.StatusSeeOther)
}

func (h *Handler) handleDeleteIntegration(w http.ResponseWriter, r *http.Request, integrationID string) {
	lang := h.determineLanguage(r)
	if err := h.Store.DeleteIntegration(integrationID); err != nil {
		h.renderError(w, "integrations.html", h.I18n.Sprintf(lang, "Failed to delete integration: %s", err.Error()), http.StatusInternalServerError, r)
		return
	}

	h.Logger.Info("integration deleted successfully", "integration_id", integrationID)
	http.Redirect(w, r, "/admin/integrations?status=deleted", http.StatusSeeOther)
}


// generateMermaidDiagram creates a Mermaid.js graph definition string.
func generateMermaidDiagram(collectors []storage.Collector, routes []storage.RouteInfo, collectorLabel string) string {
	var sb bytes.Buffer
	sb.WriteString("graph TD\n")

	// Define styles
	sb.WriteString("    classDef collector fill:#f9f,stroke:#333,stroke-width:2px;\n")
	sb.WriteString("    classDef channel fill:#ccf,stroke:#333,stroke-width:2px;\n")
    sb.WriteString("    classDef transform fill:#9f9,stroke:#333,stroke-width:2px;\n")


	// Define collector nodes
	for _, c := range collectors {
		sb.WriteString(fmt.Sprintf(`    C%s["`+collectorLabel+`"]:::collector`+"\n", c.ID, template.HTMLEscapeString(c.Name)))
	}

	// Create a map to avoid duplicating channel nodes
	channelNodes := make(map[string]storage.RouteInfo)
	for _, r := range routes {
		// Only add actual channels, not collector outputs
		if !strings.HasPrefix(r.SourceChannelID, "collector-output:") {
			channelNodes[r.SourceChannelID] = r
		}
		if r.DestinationChannelID != "" {
			channelNodes[r.DestinationChannelID] = r
		}
	}

	// Add channel nodes from sources and destinations
	for id, r := range channelNodes {
		name := r.SourceChannelName
		app := r.SourceAppName

		if r.DestinationChannelID != "" && id == r.DestinationChannelID {
			name = r.DestinationChannelName
			app = r.DestinationAppName
		}

		// Use the ID from the map key to ensure uniqueness
		sb.WriteString(fmt.Sprintf(`    CH%s["(%s)<br>%s"]:::channel`+"\n", id, template.HTMLEscapeString(app), template.HTMLEscapeString(name)))
	}


	if len(routes) == 0 && len(collectors) > 0 {
		// Just show collectors if there are no routes
		return sb.String()
	}

	// Define links based on routes
	for _, r := range routes {
		sourceNode := fmt.Sprintf("CH%s", r.SourceChannelID)
		if strings.HasPrefix(r.SourceChannelID, "collector-output:") {
			collectorID := strings.TrimPrefix(r.SourceChannelID, "collector-output:")
			sourceNode = fmt.Sprintf("C%s", collectorID)
		}

		destNode := fmt.Sprintf("CH%s", r.DestinationChannelID)

		if r.TransformationID != "" {
			transformNode := fmt.Sprintf("T%s", r.TransformationID)
			sb.WriteString(fmt.Sprintf(`    %s["%s"]:::transform`+"\n", transformNode, template.HTMLEscapeString(r.TransformationName)))
			sb.WriteString(fmt.Sprintf("    %s --> %s --> %s\n", sourceNode, transformNode, destNode))
		} else {
			sb.WriteString(fmt.Sprintf("    %s -->|%s| %s\n", sourceNode, r.RouteType, destNode))
		}
	}

	return sb.String()
}
