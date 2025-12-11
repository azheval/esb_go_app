package admin

import (
	"net/http"

	"github.com/google/uuid"

	"esb-go-app/storage"
)

// TransformationRoutes handles routing for /admin/transformations/* paths.
func TransformationRoutes(h *Handler, w http.ResponseWriter, r *http.Request, parts []string) {
	if r.Method == http.MethodGet {
		if len(parts) == 0 || (len(parts) == 1 && parts[0] == "") {
			h.handleListTransformations(w, r)
			return
		}
		if len(parts) == 1 {
			transformationID := parts[0]
			h.handleViewTransformation(w, r, transformationID)
			return
		}
	}

	if r.Method == http.MethodPost {
		if len(parts) == 1 && parts[0] == "create" {
			h.handleCreateTransformation(w, r)
			return
		}
		if len(parts) == 2 && parts[0] == "update" {
			transformationID := parts[1]
			h.handleUpdateTransformation(w, r, transformationID)
			return
		}
		if len(parts) == 2 && parts[1] == "delete" {
			transformationID := parts[0]
			h.handleDeleteTransformation(w, r, transformationID)
			return
		}
	}

	http.NotFound(w, r)
}

func (h *Handler) handleListTransformations(w http.ResponseWriter, r *http.Request) {
	transformations, err := h.Store.GetAllTransformations()
	if err != nil {
		h.renderError(w, "transformations.html", "Failed to retrieve transformations: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := PageData{
		Transformations: transformations,
	}

	status := r.URL.Query().Get("status")
	if status == "created" {
		data.StatusMessage = "Трансформация успешно создана!"
	} else if status == "deleted" {
		data.StatusMessage = "Трансформация удалена."
	} else if status == "updated" {
		data.StatusMessage = "Трансформация успешно обновлена."
	}

	h.renderTemplate(w, "transformations.html", data)
}

func (h *Handler) handleViewTransformation(w http.ResponseWriter, r *http.Request, transformationID string) {
	transformation, err := h.Store.GetTransformationByID(transformationID)
	if err != nil {
		h.renderError(w, "transformations.html", "Failed to retrieve transformation: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if transformation == nil {
		h.renderError(w, "transformations.html", "Transformation not found.", http.StatusNotFound)
		return
	}

	data := PageData{
		Transformation: transformation,
	}

	h.renderTemplate(w, "transformation_details.html", data)
}

func (h *Handler) handleCreateTransformation(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderError(w, "transformations.html", "Failed to parse form.", http.StatusBadRequest)
		return
	}

	transformation := &storage.Transformation{
		ID:     uuid.New().String(),
		Name:   r.FormValue("name"),
		Engine: r.FormValue("engine"),
		Script: r.FormValue("script"),
	}

	if transformation.Name == "" || transformation.Engine == "" || transformation.Script == "" {
		h.renderError(w, "transformations.html", "Name, engine, and script are required.", http.StatusBadRequest)
		return
	}

	if err := h.Store.CreateTransformation(transformation); err != nil {
		h.renderError(w, "transformations.html", "Failed to create transformation: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.Logger.Info("transformation created successfully", "transformation_name", transformation.Name, "transformation_id", transformation.ID)
	http.Redirect(w, r, "/admin/transformations?status=created", http.StatusSeeOther)
}

func (h *Handler) handleUpdateTransformation(w http.ResponseWriter, r *http.Request, transformationID string) {
	if err := r.ParseForm(); err != nil {
		h.renderError(w, "transformation_details.html", "Failed to parse form.", http.StatusBadRequest)
		return
	}

	transformation := &storage.Transformation{
		ID:     transformationID,
		Name:   r.FormValue("name"),
		Engine: r.FormValue("engine"),
		Script: r.FormValue("script"),
	}

	if transformation.Name == "" || transformation.Engine == "" || transformation.Script == "" {
		h.renderError(w, "transformation_details.html", "Name, engine, and script are required.", http.StatusBadRequest)
		return
	}

	if err := h.Store.UpdateTransformation(transformation); err != nil {
		h.renderError(w, "transformation_details.html", "Failed to update transformation: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.Logger.Info("transformation updated successfully", "transformation_id", transformationID)
	http.Redirect(w, r, "/admin/transformations?status=updated", http.StatusSeeOther)
}

func (h *Handler) handleDeleteTransformation(w http.ResponseWriter, r *http.Request, transformationID string) {
	if err := h.Store.DeleteTransformation(transformationID); err != nil {
		h.renderError(w, "transformations.html", "Failed to delete transformation: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.Logger.Info("transformation deleted successfully", "transformation_id", transformationID)
	http.Redirect(w, r, "/admin/transformations?status=deleted", http.StatusSeeOther)
}
