package complaints

import (
	"net/http"
	"strings"

	"modular-api/internal/platform/httpserver"
)

type Handler struct {
	module *Module
}

func (m *Module) Handler() *Handler {
	return &Handler{module: m}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/admin/complaints", h.handleAdminCollection)
	mux.HandleFunc("/api/v1/admin/complaints/", h.handleAdminItem)
	mux.HandleFunc("/api/v1/resident-complaints", h.handleResidentCollection)
	mux.HandleFunc("/api/v1/resident-complaints/", h.handleResidentItem)

	mux.HandleFunc("/api/v1/complaints", h.handleCollection)
	mux.HandleFunc("/api/v1/complaints/", h.handleItem)
}

func (h *Handler) handleCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.writeComplaintList(w, r)
	case http.MethodPost:
		h.createResidentComplaint(w, r)
	default:
		httpserver.WriteMethodNotAllowed(w)
	}
}

func (h *Handler) handleResidentCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.writeComplaintList(w, r)
	case http.MethodPost:
		h.createResidentComplaint(w, r)
	default:
		httpserver.WriteMethodNotAllowed(w)
	}
}

func (h *Handler) handleAdminCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	h.writeComplaintList(w, r)
}

func (h *Handler) handleItem(w http.ResponseWriter, r *http.Request) {
	path, ok := complaintPath(r.URL.Path, "/api/v1/complaints/")
	if !ok {
		httpserver.WriteError(w, http.StatusBadRequest, "complaint id is required")
		return
	}

	if complaintID, ok := complaintStatusPath(path); ok {
		h.handleStatusUpdate(w, r, complaintID)
		return
	}

	if r.Method != http.MethodGet {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	item, err := h.module.Get(path)
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

	publicID, ok := httpserver.PathValue(r.URL.Path, "/api/v1/resident-complaints/")
	if !ok {
		httpserver.WriteError(w, http.StatusBadRequest, "complaint id is required")
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
	path, ok := complaintPath(r.URL.Path, "/api/v1/admin/complaints/")
	if !ok {
		httpserver.WriteError(w, http.StatusBadRequest, "complaint id is required")
		return
	}

	if complaintID, ok := complaintStatusPath(path); ok {
		h.handleStatusUpdate(w, r, complaintID)
		return
	}

	if r.Method != http.MethodGet {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	item, err := h.module.Get(path)
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteItem(w, *item)
}

func (h *Handler) handleStatusUpdate(w http.ResponseWriter, r *http.Request, publicID string) {
	if r.Method != http.MethodPatch {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	var input UpdateComplaintStatusRequest
	if err := httpserver.Decode(w, r, &input); err != nil {
		return
	}

	item, err := h.module.UpdateStatus(publicID, input.Status, input.Comment)
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteItem(w, *item)
}

func (h *Handler) writeComplaintList(w http.ResponseWriter, r *http.Request) {
	items, err := h.module.List(r.URL.Query().Get("unitCode"))
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteOK(w, ComplaintListResponse{Items: items})
}

func (h *Handler) createResidentComplaint(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		httpserver.WriteError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	item, err := h.module.Create(r.Context(), CreateComplaintInput{
		AccountCode:       r.FormValue("accountCode"),
		UnitCode:          r.FormValue("unitCode"),
		Category:          r.FormValue("category"),
		Title:             r.FormValue("title"),
		Description:       r.FormValue("description"),
		Location:          r.FormValue("location"),
		Priority:          r.FormValue("priority"),
		AttachmentHeaders: r.MultipartForm.File["attachments"],
	})
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteCreatedItem(w, "/api/v1/resident-complaints/"+item.ID, *item)
}

func complaintPath(path, prefix string) (string, bool) {
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}

	value := strings.TrimPrefix(path, prefix)
	if value == "" || value == path {
		return "", false
	}

	return value, true
}

func complaintStatusPath(value string) (string, bool) {
	if !strings.HasSuffix(value, "/status") {
		return "", false
	}

	complaintID := strings.TrimSuffix(value, "/status")
	if complaintID == "" || strings.Contains(complaintID, "/") {
		return "", false
	}

	return complaintID, true
}
