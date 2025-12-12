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
	lang := h.determineLanguage(r)
	transformations, err := h.Store.GetAllTransformations()
	if err != nil {
		h.renderError(w, "transformations.html", h.I18n.Sprintf(lang, "Failed to retrieve transformations: %s", err.Error()), http.StatusInternalServerError, r)
		return
	}

	data := PageData{
		Transformations: transformations,
		AcceptLanguage:  lang,
	}

	status := r.URL.Query().Get("status")
	if status == "created" {
		data.StatusMessage = h.I18n.Sprintf(lang, "Transformation created successfully!")
	} else if status == "deleted" {
		data.StatusMessage = h.I18n.Sprintf(lang, "Transformation deleted.")
	} else if status == "updated" {
		data.StatusMessage = h.I18n.Sprintf(lang, "Transformation updated successfully!")
	}

	h.renderTemplate(w, "transformations.html", data)
}

func (h *Handler) handleViewTransformation(w http.ResponseWriter, r *http.Request, transformationID string) {
	lang := h.determineLanguage(r)
	transformation, err := h.Store.GetTransformationByID(transformationID)
	if err != nil {
		h.renderError(w, "transformations.html", h.I18n.Sprintf(lang, "Failed to retrieve transformation: %s", err.Error()), http.StatusInternalServerError, r)
		return
	}
	if transformation == nil {
		h.renderError(w, "transformations.html", h.I18n.Sprintf(lang, "Transformation not found."), http.StatusNotFound, r)
		return
	}

	data := PageData{
		Transformation: transformation,
		AcceptLanguage: lang,
	}

	h.renderTemplate(w, "transformation_details.html", data)
}

func (h *Handler) handleCreateTransformation(w http.ResponseWriter, r *http.Request) {
	lang := h.determineLanguage(r)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, "transformations.html", h.I18n.Sprintf(lang, "Failed to parse form."), http.StatusBadRequest, r)
		return
	}

	transformation := &storage.Transformation{
		ID:     uuid.New().String(),
		Name:   r.FormValue("name"),
		Engine: r.FormValue("engine"),
		Script: r.FormValue("script"),
	}

	if transformation.Name == "" || transformation.Engine == "" || transformation.Script == "" {
		h.renderError(w, "transformations.html", h.I18n.Sprintf(lang, "Name, engine, and script are required."), http.StatusBadRequest, r)
		return
	}

	if err := h.Store.CreateTransformation(transformation); err != nil {
		h.renderError(w, "transformations.html", h.I18n.Sprintf(lang, "Failed to create transformation: %s", err.Error()), http.StatusInternalServerError, r)
		return
	}

	h.Logger.Info("transformation created successfully", "transformation_name", transformation.Name, "transformation_id", transformation.ID)
	http.Redirect(w, r, "/admin/transformations?status=created", http.StatusSeeOther)
}

func (h *Handler) handleUpdateTransformation(w http.ResponseWriter, r *http.Request, transformationID string) {
	lang := h.determineLanguage(r)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, "transformation_details.html", h.I18n.Sprintf(lang, "Failed to parse form."), http.StatusBadRequest, r)
		return
	}

	transformation := &storage.Transformation{
		ID:     transformationID,
		Name:   r.FormValue("name"),
		Engine: r.FormValue("engine"),
		Script: r.FormValue("script"),
	}

	if transformation.Name == "" || transformation.Engine == "" || transformation.Script == "" {
		h.renderError(w, "transformation_details.html", h.I18n.Sprintf(lang, "Name, engine, and script are required."), http.StatusBadRequest, r)
		return
	}

	if err := h.Store.UpdateTransformation(transformation); err != nil {
		h.renderError(w, "transformation_details.html", h.I18n.Sprintf(lang, "Failed to update transformation: %s", err.Error()), http.StatusInternalServerError, r)
		return
	}

	h.Logger.Info("transformation updated successfully", "transformation_id", transformationID)
	http.Redirect(w, r, "/admin/transformations?status=updated", http.StatusSeeOther)
}

func (h *Handler) handleDeleteTransformation(w http.ResponseWriter, r *http.Request, transformationID string) {
	lang := h.determineLanguage(r)
	if err := h.Store.DeleteTransformation(transformationID); err != nil {
		h.renderError(w, "transformations.html", h.I18n.Sprintf(lang, "Failed to delete transformation: %s", err.Error()), http.StatusInternalServerError, r)
		return
	}

	h.Logger.Info("transformation deleted successfully", "transformation_id", transformationID)
	http.Redirect(w, r, "/admin/transformations?status=deleted", http.StatusSeeOther)
}
