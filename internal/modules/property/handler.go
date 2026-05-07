package property

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
	mux.HandleFunc("/api/v1/property/tree", h.handleTree)
	mux.HandleFunc("/api/v1/property/units", h.handleUnits)
	mux.HandleFunc("/api/v1/property/resident-accounts", h.handleResidentAccounts)
	mux.HandleFunc("/api/v1/property/owner-tenants", h.handleOwnerTenants)
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

func (h *Handler) handleOwnerTenants(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.module.OwnerTenantRegistrations(
			strings.Split(r.URL.Query().Get("ownerAccountCodes"), ","),
			r.URL.Query().Get("unitCode"),
		)
		if err != nil {
			httpserver.WriteMappedError(w, err)
			return
		}

		httpserver.WriteOK(w, map[string]any{"items": items})
	case http.MethodPost:
		var input struct {
			OwnerAccountCode  string `json:"ownerAccountCode"`
			OwnerResidentCode string `json:"ownerResidentCode"`
			OwnerName         string `json:"ownerName"`
			PropertyName      string `json:"propertyName"`
			UnitNumber        string `json:"unitNumber"`
			TenantName        string `json:"tenantName"`
			TenantEmail       string `json:"tenantEmail"`
			TenantPhone       string `json:"tenantPhone"`
			MoveInDate        string `json:"moveInDate"`
			Notes             string `json:"notes"`
		}
		if err := httpserver.Decode(w, r, &input); err != nil {
			return
		}

		item, err := h.module.RegisterOwnerTenant(RegisterOwnerTenantInput{
			OwnerAccountCode:  input.OwnerAccountCode,
			OwnerResidentCode: input.OwnerResidentCode,
			OwnerName:         input.OwnerName,
			PropertyName:      input.PropertyName,
			UnitNumber:        input.UnitNumber,
			TenantName:        input.TenantName,
			TenantEmail:       input.TenantEmail,
			TenantPhone:       input.TenantPhone,
			MoveInDate:        input.MoveInDate,
			Notes:             input.Notes,
		})
		if err != nil {
			httpserver.WriteMappedError(w, err)
			return
		}

		httpserver.WriteCreatedItem(w, "/api/v1/property/owner-tenants/"+item.ID, *item)
	default:
		httpserver.WriteMethodNotAllowed(w)
	}
}
