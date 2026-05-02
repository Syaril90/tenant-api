package feedback

import (
	"net/http"

	"modular-api/internal/platform/httpserver"
)

type Handler struct {
	module *Module
}

func (m *Module) Handler() *Handler {
	return &Handler{module: m}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/admin/feedback", h.handleAdminCollection)
	mux.HandleFunc("/api/v1/admin/feedback/", h.handleAdminItem)
	mux.HandleFunc("/api/v1/feedback", h.handleCollection)
	mux.HandleFunc("/api/v1/feedback/", h.handleItem)
	mux.HandleFunc("/api/v1/resident-feedback", h.handleResidentCollection)
	mux.HandleFunc("/api/v1/resident-feedback/", h.handleResidentItem)
}

func (h *Handler) handleCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.writeFeedbackList(w, r)
	case http.MethodPost:
		h.createResidentFeedback(w, r)
	default:
		httpserver.WriteMethodNotAllowed(w)
	}
}

func (h *Handler) handleResidentCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.writeFeedbackList(w, r)
	case http.MethodPost:
		h.createResidentFeedback(w, r)
	default:
		httpserver.WriteMethodNotAllowed(w)
	}
}

func (h *Handler) handleAdminCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	h.writeFeedbackList(w, r)
}

func (h *Handler) handleItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	publicID, ok := httpserver.PathValue(r.URL.Path, "/api/v1/feedback/")
	if !ok {
		httpserver.WriteError(w, http.StatusBadRequest, "feedback id is required")
		return
	}

	item, err := h.module.Get(publicID)
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteItem(w, *item)
}

func (h *Handler) handleAdminItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	publicID, ok := httpserver.PathValue(r.URL.Path, "/api/v1/admin/feedback/")
	if !ok {
		httpserver.WriteError(w, http.StatusBadRequest, "feedback id is required")
		return
	}

	item, err := h.module.Get(publicID)
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteItem(w, *item)
}

func (h *Handler) handleResidentItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	publicID, ok := httpserver.PathValue(r.URL.Path, "/api/v1/resident-feedback/")
	if !ok {
		httpserver.WriteError(w, http.StatusBadRequest, "feedback id is required")
		return
	}

	item, err := h.module.Get(publicID)
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteItem(w, *item)
}

func (h *Handler) writeFeedbackList(w http.ResponseWriter, r *http.Request) {
	items, err := h.module.List(r.URL.Query().Get("unitCode"))
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteOK(w, FeedbackListResponse{Items: items})
}

func (h *Handler) createResidentFeedback(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		httpserver.WriteError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	item, err := h.module.Create(r.Context(), CreateFeedbackInput{
		AccountCode:       r.FormValue("accountCode"),
		UnitCode:          r.FormValue("unitCode"),
		Type:              r.FormValue("type"),
		Rating:            r.FormValue("rating"),
		Details:           r.FormValue("details"),
		AttachmentHeaders: r.MultipartForm.File["attachments"],
	})
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteCreatedItem(w, "/api/v1/resident-feedback/"+item.ID, *item)
}
