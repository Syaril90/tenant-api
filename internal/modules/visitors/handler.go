package visitors

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
	mux.HandleFunc("/api/v1/visitor-requests", h.handleVisitors)
	mux.HandleFunc("/api/v1/admin/visitor-requests", h.handleAdminVisitorRequests)
	mux.HandleFunc("/api/v1/admin/visitor-parking-configs", h.handleAdminParkingConfigs)
	mux.HandleFunc("/api/v1/admin/visitor-requests/workspace", h.handleAdminWorkspace)
	mux.HandleFunc("/api/v1/visitors", h.handleVisitors)
	mux.HandleFunc("/api/v1/visitors/admin", h.handleAdminWorkspace)
}

func (h *Handler) handleVisitors(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.module.List(r.URL.Query().Get("unitCode"))
		if err != nil {
			httpserver.WriteMappedError(w, err)
			return
		}

		httpserver.WriteOK(w, map[string]any{"items": items})
	case http.MethodPost:
		var input CreateVisitorRequest
		if err := httpserver.Decode(w, r, &input); err != nil {
			return
		}

		item, err := h.module.Create(CreateVisitorInput{
			AccountCode:           input.AccountCode,
			UnitCode:              input.UnitCode,
			VisitorName:           input.VisitorName,
			Purpose:               input.Purpose,
			VisitDate:             input.VisitDate,
			ArrivalWindow:         input.ArrivalWindow,
			VehiclePlate:          input.VehiclePlate,
			ParkingSlotsRequested: input.ParkingSlotsRequested,
		})
		if err != nil {
			httpserver.WriteMappedError(w, err)
			return
		}

		httpserver.WriteCreatedItem(w, "/api/v1/visitor-requests/"+item.ID, item)
	default:
		httpserver.WriteMethodNotAllowed(w)
	}
}

func (h *Handler) handleAdminWorkspace(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		workspace, err := h.module.AdminWorkspace()
		if err != nil {
			httpserver.WriteMappedError(w, err)
			return
		}

		httpserver.WriteOK(w, workspace)
	case http.MethodPut:
		var input struct {
			Approvals       []AdminApprovalInput       `json:"approvals"`
			BuildingConfigs []AdminBuildingConfigInput `json:"buildingConfigs"`
		}
		if err := httpserver.Decode(w, r, &input); err != nil {
			return
		}

		workspace, err := h.module.ReplaceAdminWorkspace(input.Approvals, input.BuildingConfigs)
		if err != nil {
			httpserver.WriteMappedError(w, err)
			return
		}

		httpserver.WriteOK(w, workspace)
	default:
		httpserver.WriteMethodNotAllowed(w)
	}
}

func (h *Handler) handleAdminVisitorRequests(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.module.AdminVisitorRequests()
		if err != nil {
			httpserver.WriteMappedError(w, err)
			return
		}

		httpserver.WriteOK(w, VisitorRequestsResponse{Items: items})
	case http.MethodPut:
		var input ReplaceVisitorRequestsRequest
		if err := httpserver.Decode(w, r, &input); err != nil {
			return
		}

		items, err := h.module.ReplaceAdminVisitorRequests(input.Items)
		if err != nil {
			httpserver.WriteMappedError(w, err)
			return
		}

		httpserver.WriteOK(w, VisitorRequestsResponse{Items: items})
	default:
		httpserver.WriteMethodNotAllowed(w)
	}
}

func (h *Handler) handleAdminParkingConfigs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.module.AdminParkingConfigs()
		if err != nil {
			httpserver.WriteMappedError(w, err)
			return
		}

		httpserver.WriteOK(w, ParkingConfigsResponse{Items: items})
	case http.MethodPut:
		var input ReplaceParkingConfigsRequest
		if err := httpserver.Decode(w, r, &input); err != nil {
			return
		}

		items, err := h.module.ReplaceAdminParkingConfigs(input.Items)
		if err != nil {
			httpserver.WriteMappedError(w, err)
			return
		}

		httpserver.WriteOK(w, ParkingConfigsResponse{Items: items})
	default:
		httpserver.WriteMethodNotAllowed(w)
	}
}
