package documents

import (
	"net/http"
	"strings"

	"modular-api/internal/platform/httpserver"
)

func (h *Handler) handleAdminDocumentRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	items, err := h.module.ListAdminRequests()
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteOK(w, DocumentRequestListPayload{Items: items})
}

func (h *Handler) handleResidentDocumentRequests(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.module.ListResidentRequests(
			r.URL.Query().Get("accountCode"),
			r.URL.Query().Get("unitCode"),
		)
		if err != nil {
			httpserver.WriteMappedError(w, err)
			return
		}

		httpserver.WriteOK(w, DocumentRequestListPayload{Items: items})
	case http.MethodPost:
		if err := r.ParseMultipartForm(12 << 20); err != nil {
			httpserver.WriteError(w, http.StatusBadRequest, "invalid multipart form")
			return
		}

		item, err := h.module.CreateRequest(r.Context(), CreateDocumentRequestInput{
			AccountCode:       r.FormValue("accountCode"),
			UnitCode:          r.FormValue("unitCode"),
			RequestTypeID:     r.FormValue("typeId"),
			Purpose:           r.FormValue("purpose"),
			PreferredFormatID: r.FormValue("preferredFormatId"),
			Notes:             r.FormValue("notes"),
			AttachmentHeaders: r.MultipartForm.File["attachments"],
		})
		if err != nil {
			httpserver.WriteMappedError(w, err)
			return
		}

		httpserver.WriteCreatedItem(w, "/api/v1/document-requests/"+item.ID, *item)
	default:
		httpserver.WriteMethodNotAllowed(w)
	}
}

func (h *Handler) handleAdminDocumentRequestItem(w http.ResponseWriter, r *http.Request) {
	path, ok := documentRequestPath(r.URL.Path, "/api/v1/admin/document-requests/")
	if !ok {
		httpserver.WriteError(w, http.StatusBadRequest, "document request id is required")
		return
	}

	requestID, ok := documentRequestStatusPath(path)
	if !ok {
		httpserver.WriteError(w, http.StatusBadRequest, "document request status path is required")
		return
	}

	if r.Method != http.MethodPatch {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	if err := r.ParseMultipartForm(20 << 20); err != nil {
		httpserver.WriteError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	item, err := h.module.UpdateRequest(r.Context(), requestID, UpdateDocumentRequestInput{
		Status:            r.FormValue("status"),
		Comment:           r.FormValue("comment"),
		AttachmentHeaders: r.MultipartForm.File["attachments"],
	})
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteItem(w, *item)
}

func documentRequestPath(path, prefix string) (string, bool) {
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}

	value := strings.TrimPrefix(path, prefix)
	if value == "" || value == path {
		return "", false
	}

	return value, true
}

func documentRequestStatusPath(value string) (string, bool) {
	if !strings.HasSuffix(value, "/status") {
		return "", false
	}

	requestID := strings.TrimSuffix(value, "/status")
	if requestID == "" || strings.Contains(requestID, "/") {
		return "", false
	}

	return requestID, true
}
