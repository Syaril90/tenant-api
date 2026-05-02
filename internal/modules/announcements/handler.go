package announcements

import (
	"mime/multipart"
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
	mux.HandleFunc("/api/v1/announcements", h.handleList)
	mux.HandleFunc("/api/v1/announcements/", h.handleGet)
	mux.HandleFunc("/api/v1/admin/announcements", h.handleCreate)
	mux.HandleFunc("/api/v1/announcements/admin", h.handleCreate)
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	items, err := h.module.ListPublished()
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteOK(w, map[string]any{"items": items})
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	publicID, ok := httpserver.PathValue(r.URL.Path, "/api/v1/announcements/")
	if !ok {
		httpserver.WriteError(w, http.StatusBadRequest, "announcement id is required")
		return
	}

	item, err := h.module.GetPublished(publicID)
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteItem(w, *item)
}

func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		httpserver.WriteError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	var imageFile multipart.File
	var imageHeader *multipart.FileHeader
	file, header, err := r.FormFile("image")
	if err == nil {
		imageFile = file
		imageHeader = header
		defer imageFile.Close()
	}

	attachmentHeaders := r.MultipartForm.File["attachments"]

	item, err := h.module.Create(r.Context(), CreateAnnouncementInput{
		Title:             r.FormValue("title"),
		Description:       r.FormValue("description"),
		BadgeTone:         r.FormValue("badgeTone"),
		AffectedArea:      r.FormValue("affectedArea"),
		EffectiveAt:       r.FormValue("effectiveAt"),
		Contact:           r.FormValue("contact"),
		ImageFile:         imageFile,
		ImageHeader:       imageHeader,
		AttachmentHeaders: attachmentHeaders,
	})
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteCreatedItem(w, "/api/v1/announcements/"+item.ID, *item)
}
