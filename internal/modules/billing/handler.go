package billing

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
	mux.HandleFunc("/api/v1/admin/billing/tree", h.handleAdminTree)
	mux.HandleFunc("/api/v1/admin/billing/charges", h.handleCreateCharge)
	mux.HandleFunc("/api/v1/admin/billing/payments", h.handleRecordPayment)
	mux.HandleFunc("/api/v1/billing/billplz/checkout", h.handleCreateBillplzCheckout)
	mux.HandleFunc("/api/v1/billing/billplz/confirm", h.handleConfirmBillplzPayment)
	mux.HandleFunc("/api/v1/billing/payments/billplz/callback", h.handleBillplzCallback)
	mux.HandleFunc("/api/v1/resident/billing/", h.handleResidentBilling)
	mux.HandleFunc("/api/v1/billing/admin/tree", h.handleAdminTree)
	mux.HandleFunc("/api/v1/billing/admin/charges", h.handleCreateCharge)
	mux.HandleFunc("/api/v1/billing/admin/payments", h.handleRecordPayment)
	mux.HandleFunc("/api/v1/billing/resident/", h.handleResidentBilling)
}

func (h *Handler) handleAdminTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	tree, err := h.module.AdminTree()
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteOK(w, map[string]any{"items": tree})
}

func (h *Handler) handleCreateCharge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	var input CreateChargeInput
	if err := httpserver.Decode(w, r, &input); err != nil {
		return
	}

	if err := h.module.CreateCharge(input); err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteCreated(w, "/api/v1/admin/billing/charges", map[string]string{"status": "created"})
}

func (h *Handler) handleRecordPayment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	var input RecordPaymentInput
	if err := httpserver.Decode(w, r, &input); err != nil {
		return
	}

	if err := h.module.RecordPayment(input); err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteCreated(w, "/api/v1/admin/billing/payments", map[string]string{"status": "recorded"})
}

func (h *Handler) handleResidentBilling(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	unitCode, ok := httpserver.PathValue(r.URL.Path, "/api/v1/billing/resident/")
	if !ok {
		unitCode, ok = httpserver.PathValue(r.URL.Path, "/api/v1/resident/billing/")
	}
	if !ok {
		httpserver.WriteError(w, http.StatusBadRequest, "unitCode path parameter is required")
		return
	}

	view, err := h.module.ResidentBilling(unitCode)
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteItem(w, *view)
}

func (h *Handler) handleCreateBillplzCheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	var input CreateBillplzCheckoutInput
	if err := httpserver.Decode(w, r, &input); err != nil {
		return
	}

	checkout, err := h.module.CreateBillplzCheckout(input)
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteCreatedItem(w, "/api/v1/billing/billplz/checkout", *checkout)
}

func (h *Handler) handleBillplzCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	contentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") || strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseForm(); err != nil {
			httpserver.WriteError(w, http.StatusBadRequest, "invalid Billplz callback payload")
			return
		}
	} else if err := r.ParseForm(); err != nil {
		httpserver.WriteError(w, http.StatusBadRequest, "invalid Billplz callback payload")
		return
	}

	if err := h.module.HandleBillplzCallback(r.PostForm); err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteOK(w, map[string]string{"status": "ok"})
}

func (h *Handler) handleConfirmBillplzPayment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpserver.WriteMethodNotAllowed(w)
		return
	}

	var input ConfirmBillplzPaymentInput
	if err := httpserver.Decode(w, r, &input); err != nil {
		return
	}

	status, err := h.module.ConfirmBillplzPayment(input)
	if err != nil {
		httpserver.WriteMappedError(w, err)
		return
	}

	httpserver.WriteItem(w, *status)
}
