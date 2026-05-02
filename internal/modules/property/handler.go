package property

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
	mux.HandleFunc("/api/v1/property/tree", h.handleTree)
	mux.HandleFunc("/api/v1/property/units", h.handleUnits)
	mux.HandleFunc("/api/v1/property/resident-accounts", h.handleResidentAccounts)
}

func (h *Handler) handleTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	tree, err := h.module.Tree()
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteOK(w, map[string]any{"items": tree})
}

func (h *Handler) handleUnits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	units, err := h.module.Units()
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteOK(w, map[string]any{"items": units})
}

func (h *Handler) handleResidentAccounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	items, err := h.module.ResidentAccountsByEmail(r.URL.Query().Get("email"))
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteOK(w, map[string]any{"items": items})
}
