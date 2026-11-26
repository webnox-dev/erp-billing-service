package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"erp-billing-service/internal/application"
	"erp-billing-service/internal/domain"

	"github.com/google/uuid"
)

type InvoiceSettingsHandler struct {
	service *application.InvoiceSettingsService
}

func NewInvoiceSettingsHandler(service *application.InvoiceSettingsService) *InvoiceSettingsHandler {
	return &InvoiceSettingsHandler{service: service}
}

func (h *InvoiceSettingsHandler) GetInvoiceSettings(w http.ResponseWriter, r *http.Request) {
	orgID := resolveOrganizationID(r)
	if orgID == uuid.Nil {
		http.Error(w, "organization_id is required", http.StatusBadRequest)
		return
	}

	settings, err := h.service.GetSettings(withGatewayContext(r), orgID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(settings)
}

func (h *InvoiceSettingsHandler) UpdateInvoiceSettings(w http.ResponseWriter, r *http.Request) {
	orgID := resolveOrganizationID(r)
	if orgID == uuid.Nil {
		http.Error(w, "organization_id is required", http.StatusBadRequest)
		return
	}

	var req domain.InvoiceSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	updated, err := h.service.UpdateSettings(withGatewayContext(r), orgID, &req)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(err.Error(), "validation") || strings.Contains(err.Error(), "required") {
			w.WriteHeader(http.StatusBadRequest)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}
