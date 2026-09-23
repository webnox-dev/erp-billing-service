package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"erp-billing-service/internal/application"
	"erp-billing-service/internal/application/dto"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type PaymentHandler struct {
	service *application.PaymentService
}

func NewPaymentHandler(service *application.PaymentService) *PaymentHandler {
	return &PaymentHandler{service: service}
}

// RecordPayment handles POST /api/v1/billing/payments
func (h *PaymentHandler) RecordPayment(w http.ResponseWriter, r *http.Request) {
	var req dto.RecordPaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	payment, err := h.service.RecordPayment(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(payment)
}

// VoidPayment handles POST /api/v1/billing/payments/{id}/void
func (h *PaymentHandler) VoidPayment(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	paymentID, err := uuid.Parse(vars["id"])
	if err != nil {
		http.Error(w, "invalid payment ID", http.StatusBadRequest)
		return
	}

	var req dto.VoidPaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.VoidPayment(r.Context(), paymentID, req.Notes); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListPayments handles GET /api/v1/billing/payments
func (h *PaymentHandler) ListPayments(w http.ResponseWriter, r *http.Request) {
	orgIDStr := r.Header.Get("X-Organization-ID")
	orgID, _ := uuid.Parse(orgIDStr)

	filterParams := dto.PaymentFilterParams{}

	// Module filter
	if mod := r.URL.Query().Get("module"); mod != "" {
		upperMod := strings.ToUpper(mod)
		if upperMod == "IMS" {
			upperMod = "INVENTORY"
		}
		if upperMod != "FSM" {
			filterParams.Module = &upperMod
		}
	}

	// Status filter
	if status := r.URL.Query().Get("status"); status != "" {
		filterParams.Status = &status
	}

	// Customer filter
	if custIDStr := r.URL.Query().Get("customer_id"); custIDStr != "" {
		if cid, err := uuid.Parse(custIDStr); err == nil {
			filterParams.CustomerID = &cid
		}
	}
	if custName := r.URL.Query().Get("customer_name"); custName != "" {
		filterParams.CustomerName = &custName
	}

	// Method filter
	if method := r.URL.Query().Get("method"); method != "" {
		filterParams.Method = &method
	} else if mode := r.URL.Query().Get("payment_mode"); mode != "" {
		filterParams.Method = &mode
	}

	// Search filter
	if search := r.URL.Query().Get("search"); search != "" {
		filterParams.Search = &search
	}

	// Invoice number filter
	if invNum := r.URL.Query().Get("invoice_number"); invNum != "" {
		filterParams.InvoiceNumber = &invNum
	}

	// Payment date filters
	if pStart := r.URL.Query().Get("payment_date_start"); pStart != "" {
		if t, err := time.Parse("2006-01-02", pStart); err == nil {
			filterParams.PaymentDateStart = &t
		} else if t, err := time.Parse(time.RFC3339, pStart); err == nil {
			filterParams.PaymentDateStart = &t
		}
	}
	if pEnd := r.URL.Query().Get("payment_date_end"); pEnd != "" {
		if t, err := time.Parse("2006-01-02", pEnd); err == nil {
			endOfDay := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, time.UTC)
			filterParams.PaymentDateEnd = &endOfDay
		} else if t, err := time.Parse(time.RFC3339, pEnd); err == nil {
			filterParams.PaymentDateEnd = &t
		}
	}

	// Amount range filters
	if minAmountStr := r.URL.Query().Get("min_amount"); minAmountStr != "" {
		if val, err := strconv.ParseFloat(minAmountStr, 64); err == nil {
			filterParams.MinAmount = &val
		}
	}
	if maxAmountStr := r.URL.Query().Get("max_amount"); maxAmountStr != "" {
		if val, err := strconv.ParseFloat(maxAmountStr, 64); err == nil {
			filterParams.MaxAmount = &val
		}
	}

	// Created at date filters
	if start := r.URL.Query().Get("created_at_start"); start != "" {
		if t, err := time.Parse("2006-01-02", start); err == nil {
			filterParams.CreatedAtStart = &t
		} else if t, err := time.Parse(time.RFC3339, start); err == nil {
			filterParams.CreatedAtStart = &t
		}
	} else if start := r.URL.Query().Get("start_date"); start != "" {
		if t, err := time.Parse("2006-01-02", start); err == nil {
			filterParams.CreatedAtStart = &t
		} else if t, err := time.Parse(time.RFC3339, start); err == nil {
			filterParams.CreatedAtStart = &t
		}
	}

	if end := r.URL.Query().Get("created_at_end"); end != "" {
		if t, err := time.Parse("2006-01-02", end); err == nil {
			endOfDay := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, time.UTC)
			filterParams.CreatedAtEnd = &endOfDay
		} else if t, err := time.Parse(time.RFC3339, end); err == nil {
			filterParams.CreatedAtEnd = &t
		}
	} else if end := r.URL.Query().Get("end_date"); end != "" {
		if t, err := time.Parse("2006-01-02", end); err == nil {
			endOfDay := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, time.UTC)
			filterParams.CreatedAtEnd = &endOfDay
		} else if t, err := time.Parse(time.RFC3339, end); err == nil {
			filterParams.CreatedAtEnd = &t
		}
	}

	result, err := h.service.ListPaymentsWithFilters(r.Context(), orgID, filterParams)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": result,
	})
}

// ListPaymentsByInvoice handles GET /api/v1/billing/invoices/{id}/payments
func (h *PaymentHandler) ListPaymentsByInvoice(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	invoiceID, err := uuid.Parse(vars["id"])
	if err != nil {
		http.Error(w, "invalid invoice ID", http.StatusBadRequest)
		return
	}

	payments, err := h.service.ListPaymentsByInvoice(r.Context(), invoiceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": payments,
	})
}

// GetPayment handles GET /api/v1/billing/payments/{id}
func (h *PaymentHandler) GetPayment(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	paymentID, err := uuid.Parse(vars["id"])
	if err != nil {
		http.Error(w, "invalid payment ID", http.StatusBadRequest)
		return
	}

	payment, err := h.service.GetPayment(r.Context(), paymentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payment)
}
