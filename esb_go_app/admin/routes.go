package admin

import (
	"net/http"

	"github.com/google/uuid"

	"esb-go-app/storage"
)

// RouteRoutes handles routing for /admin/routes/* paths.
func RouteRoutes(h *Handler, w http.ResponseWriter, r *http.Request, parts []string) {
	if r.Method == http.MethodGet {
		if len(parts) == 0 || (len(parts) == 1 && parts[0] == "") {
			h.handleRoutes(w, r)
			return
		}
		if len(parts) == 1 {
			routeID := parts[0]
			h.handleViewRoute(w, r, routeID)
			return
		}
	}

	if r.Method == http.MethodPost {
		if len(parts) == 1 && parts[0] == "create" {
			h.handleCreateRoute(w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "delete" {
			routeID := parts[0]
			h.handleDeleteRoute(w, r, routeID)
			return
		}
		if len(parts) == 2 && parts[1] == "edit" {
			routeID := parts[0]
			h.handleEditRoute(w, r, routeID)
			return
		}
	}

	http.NotFound(w, r)
}

func (h *Handler) handleRoutes(w http.ResponseWriter, r *http.Request) {
	routes, err := h.Store.GetAllRoutes()
	if err != nil {
		h.renderError(w, "routes.html", "Failed to retrieve routes: "+err.Error(), http.StatusInternalServerError)
		return
	}

	routeSources, err := h.Store.GetAllRouteSources()
	if err != nil {
		h.renderError(w, "routes.html", "Failed to retrieve route sources: "+err.Error(), http.StatusInternalServerError)
		return
	}

	inbound, err := h.Store.GetAllRoutableChannels("inbound")
	if err != nil {
		h.renderError(w, "routes.html", "Failed to retrieve inbound channels: "+err.Error(), http.StatusInternalServerError)
		return
	}

	transformations, err := h.Store.GetAllTransformations()
	if err != nil {
		h.renderError(w, "routes.html", "Failed to retrieve transformations: "+err.Error(), http.StatusInternalServerError)
		return
	}

	integrations, err := h.Store.GetAllIntegrations()
	if err != nil {
		h.renderError(w, "routes.html", "Failed to retrieve integrations: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := PageData{
		Routes:          routes,
		RouteSources:    routeSources,
		InboundChannels: inbound,
		Transformations: transformations,
		Integrations:    integrations,
	}

	status := r.URL.Query().Get("status")
	if status == "created" {
		data.StatusMessage = "Маршрут успешно создан!"
	} else if status == "deleted" {
		data.StatusMessage = "Маршрут удален."
	} else if status == "created_worker_failed" {
		data.ErrorMessage = "Маршрут создан, но не удалось запустить обработчик. Проверьте логи."
	}

	h.renderTemplate(w, "routes.html", data)
}

func (h *Handler) handleViewRoute(w http.ResponseWriter, r *http.Request, routeID string) {
	rawRoute, err := h.Store.GetRouteByID(routeID)
	if err != nil || rawRoute == nil {
		h.renderError(w, "routes.html", "Route not found.", http.StatusNotFound)
		return
	}

	routeInfo, err := h.Store.BuildRouteInfo(*rawRoute)
	if err != nil {
		h.renderError(w, "routes.html", "Failed to build route details: "+err.Error(), http.StatusInternalServerError)
		return
	}

	routeSources, err := h.Store.GetAllRouteSources()
	if err != nil {
		h.renderError(w, "route_details.html", "Failed to retrieve route sources: "+err.Error(), http.StatusInternalServerError)
		return
	}

	inbound, err := h.Store.GetAllRoutableChannels("inbound")
	if err != nil {
		h.renderError(w, "route_details.html", "Failed to retrieve inbound channels: "+err.Error(), http.StatusInternalServerError)
		return
	}

	outbound, err := h.Store.GetAllRoutableChannels("outbound")
	if err != nil {
		h.renderError(w, "route_details.html", "Failed to retrieve outbound channels: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Create a unified list of destination channels as per user's request
	destinationChannels := make([]storage.ChannelInfo, 0, len(inbound)+len(outbound))
	destinationChannels = append(destinationChannels, inbound...)
	destinationChannels = append(destinationChannels, outbound...)


	transformations, err := h.Store.GetAllTransformations()
	if err != nil {
		h.renderError(w, "route_details.html", "Failed to retrieve transformations: "+err.Error(), http.StatusInternalServerError)
		return
	}

	integrations, err := h.Store.GetAllIntegrations()
	if err != nil {
		h.renderError(w, "route_details.html", "Failed to retrieve integrations: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := PageData{
		Route:               &routeInfo,
		RouteSources:        routeSources,
		InboundChannels:     inbound,
		DestinationChannels: destinationChannels,
		Transformations:     transformations,
		Integrations:        integrations,
	}

	status := r.URL.Query().Get("status")
	if status == "updated" {
		data.StatusMessage = "Маршрут успешно обновлен!"
	}

	h.renderTemplate(w, "route_details.html", data)
}

func (h *Handler) handleEditRoute(w http.ResponseWriter, r *http.Request, routeID string) {
	if r.Method == http.MethodGet {
		h.handleViewRoute(w, r, routeID)
		return
	}

	// POST request
	if err := r.ParseForm(); err != nil {
		h.renderError(w, "routes.html", "Failed to parse form.", http.StatusBadRequest)
		return
	}

	route, err := h.Store.GetRouteByID(routeID)
	if err != nil || route == nil {
		h.renderError(w, "routes.html", "Route not found to update.", http.StatusNotFound)
		return
	}

	routeName := r.FormValue("name")
	sourceID := r.FormValue("source_channel_id")
	destChannelIDValue := r.FormValue("destination_channel_id")
	routeType := r.FormValue("route_type")
	transformationIDForm := r.FormValue("transformation_id")
	integrationIDForm := r.FormValue("integration_id")

	if routeName == "" || sourceID == "" || destChannelIDValue == "" {
		h.renderError(w, "routes.html", "All fields must be filled.", http.StatusBadRequest)
		return
	}

	var transformationID *string
	if routeType == "transform" {
		if transformationIDForm == "" {
			h.renderError(w, "routes.html", "Transformation is required for this route type.", http.StatusBadRequest)
			return
		}
		transformationID = &transformationIDForm
	}

	var integrationID *string
	if integrationIDForm != "" {
		integrationID = &integrationIDForm
	}

	// Update fields
	route.Name = routeName
	route.SourceChannelID = sourceID
	route.DestinationChannelID = &destChannelIDValue
	route.RouteType = routeType
	route.TransformationID = transformationID
	route.IntegrationID = integrationID

	if err := h.Store.UpdateRoute(route); err != nil {
		h.renderError(w, "routes.html", "Failed to update route: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Restart the associated worker
	h.RabbitMQ.RestartRouter(route.ID, route.Name, sourceID)
	h.Logger.Info("Route updated and worker restarted", "route_id", routeID)

	http.Redirect(w, r, "/admin/routes/"+routeID+"?status=updated", http.StatusSeeOther)
}

func (h *Handler) handleCreateRoute(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderError(w, "routes.html", "Failed to parse form.", http.StatusBadRequest)
		return
	}

	routeName := r.FormValue("name")
	sourceID := r.FormValue("source_channel_id")
	destChannelIDValue := r.FormValue("destination_channel_id")
	routeType := r.FormValue("route_type")
	transformationIDForm := r.FormValue("transformation_id")
	integrationIDForm := r.FormValue("integration_id")

	if routeName == "" {
		h.renderError(w, "routes.html", "Имя маршрута не может быть пустым.", http.StatusBadRequest)
		return
	}
	if sourceID == "" || destChannelIDValue == "" {
		h.renderError(w, "routes.html", "Source and Destination channels must be selected.", http.StatusBadRequest)
		return
	}

	var transformationID *string
	if routeType == "transform" {
		if transformationIDForm == "" {
			h.renderError(w, "routes.html", "Transformation must be selected for transform routes.", http.StatusBadRequest)
			return
		}
		transformationID = &transformationIDForm
	}

	var integrationID *string
	if integrationIDForm != "" {
		integrationID = &integrationIDForm
	}

	route := &storage.Route{
		ID:                   uuid.New().String(),
		Name:                 routeName,
		SourceChannelID:      sourceID,
		DestinationChannelID: &destChannelIDValue,
		RouteType:            routeType,
		TransformationID:     transformationID,
		IntegrationID:        integrationID,
	}

	if err := h.Store.CreateRoute(route); err != nil {
		h.renderError(w, "routes.html", "Failed to create route: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.RabbitMQ.StartRouter(route.ID, route.Name, sourceID)

	http.Redirect(w, r, "/admin/routes?status=created", http.StatusSeeOther)
}

func (h *Handler) handleDeleteRoute(w http.ResponseWriter, r *http.Request, routeID string) {
	if err := h.Store.DeleteRoute(routeID); err != nil {
		h.renderError(w, "routes.html", "Failed to delete route: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.Logger.Info("route deleted successfully", "route_id", routeID)
	http.Redirect(w, r, "/admin/routes?status=deleted", http.StatusSeeOther)
}
