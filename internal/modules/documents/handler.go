package documents

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
	mux.HandleFunc("/api/v1/admin/documents", h.handleAdminDocuments)
	mux.HandleFunc("/api/v1/documents", h.handleResidentDocuments)
	mux.HandleFunc("/api/v1/documents/", h.handleGetDocument)
}

func (h *Handler) handleAdminDocuments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		payload, err := h.module.ListAdmin()
		if err != nil {
			httpserver.WriteMappedError(w, err)
			return
		}
		httpserver.WriteOK(w, payload)
	case http.MethodPost:
		if err := r.ParseMultipartForm(20 << 20); err != nil {
			httpserver.WriteError(w, http.StatusBadRequest, "invalid multipart form")
			return
		}

		var file multipart.File
		var header *multipart.FileHeader
		nextFile, nextHeader, err := r.FormFile("file")
		if err == nil {
			file = nextFile
			header = nextHeader
			defer file.Close()
		}

		item, err := h.module.Create(r.Context(), CreateDocumentInput{
			Title:        r.FormValue("title"),
			Description:  r.FormValue("description"),
			CategoryID:   r.FormValue("categoryId"),
			BuildingCode: r.FormValue("buildingCode"),
			File:         file,
			FileHeader:   header,
		})
		if err != nil {
			httpserver.WriteMappedError(w, err)
			return
		}

		httpserver.WriteCreatedItem(w, "/api/v1/documents/"+item.ID, *item)
	default:
		httpserver.WriteMethodNotAllowed(w)
	}
}

func (h *Handler) handleResidentDocuments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	unitCode := r.URL.Query().Get("unitCode")
	payload, err := h.module.ListResident(unitCode)
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteOK(w, payload)
}

func (h *Handler) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	documentID, ok := httpserver.PathValue(r.URL.Path, "/api/v1/documents/")
	if !ok {
		httpserver.WriteError(w, http.StatusBadRequest, "document id is required")
		return
	}

	item, err := h.module.Get(documentID)
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteItem(w, *item)
}
