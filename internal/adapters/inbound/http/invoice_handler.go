package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"erp-billing-service/internal/application"
	"erp-billing-service/internal/application/dto"
	"erp-billing-service/internal/domain"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type InvoiceHandler struct {
	service *application.InvoiceService
}

func NewInvoiceHandler(service *application.InvoiceService) *InvoiceHandler {
	return &InvoiceHandler{service: service}
}

func (h *InvoiceHandler) CreateInvoice(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fmt.Printf("[ERROR] Failed to decode invoice request: %v\n", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// In a real app, orgID would come from the auth token context
	orgIDStr := r.Header.Get("X-Organization-ID")
	orgID, _ := uuid.Parse(orgIDStr)
	if orgID == uuid.Nil {
		// Default to a zero UUID if missing, log it for debugging
		// Avoid generating random new UUIDs as that makes data unreachable
		fmt.Printf("[DEBUG] X-Organization-ID header missing in CreateInvoice, using Nil UUID\n")
	}

	fmt.Printf("[DEBUG] Creating invoice with orgID: %s, request: %+v\n", orgID, req)
	invoice, err := h.service.CreateInvoice(r.Context(), orgID, req)
	if err != nil {
		fmt.Printf("[ERROR] Failed to create invoice: %v\n", err)
		// Check if it's a validation error (stock unavailable, etc.)
		if strings.Contains(err.Error(), "stock unavailable") ||
			strings.Contains(err.Error(), "validation") ||
			strings.Contains(err.Error(), "invalid") {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(invoice)
}

func (h *InvoiceHandler) UpdateInvoice(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["id"])
	if err != nil {
		http.Error(w, "Invalid Invoice ID", http.StatusBadRequest)
		return
	}

	var req dto.CreateInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Assuming UpdateInvoice logic in service returns updated invoice or error
	invoice, err := h.service.UpdateInvoice(r.Context(), id, req)
	if err != nil {
		if err.Error() == "invoice not found" {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(invoice)
}

func (h *InvoiceHandler) ListInvoices(w http.ResponseWriter, r *http.Request) {
	orgIDStr := r.Header.Get("X-Organization-ID")
	orgID, _ := uuid.Parse(orgIDStr)

	filterParams := dto.InvoiceFilterParams{}

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

	// Payment terms filter
	if terms := r.URL.Query().Get("payment_terms"); terms != "" {
		filterParams.PaymentTerms = &terms
	}

	// Category filter
	if catIDStr := r.URL.Query().Get("category_id"); catIDStr != "" {
		if catID, err := uuid.Parse(catIDStr); err == nil {
			filterParams.CategoryID = &catID
		}
	}

	// Search filter
	if search := r.URL.Query().Get("search"); search != "" {
		filterParams.Search = &search
	}

	// Due date filters
	if dueStart := r.URL.Query().Get("due_date_start"); dueStart != "" {
		if t, err := time.Parse("2006-01-02", dueStart); err == nil {
			filterParams.DueDateStart = &t
		} else if t, err := time.Parse(time.RFC3339, dueStart); err == nil {
			filterParams.DueDateStart = &t
		}
	}

	if dueEnd := r.URL.Query().Get("due_date_end"); dueEnd != "" {
		if t, err := time.Parse("2006-01-02", dueEnd); err == nil {
			endOfDay := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, time.UTC)
			filterParams.DueDateEnd = &endOfDay
		} else if t, err := time.Parse(time.RFC3339, dueEnd); err == nil {
			filterParams.DueDateEnd = &t
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

	result, err := h.service.ListInvoicesWithFilters(r.Context(), orgID, filterParams)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": result,
	})
}

func (h *InvoiceHandler) GetInvoice(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := uuid.Parse(vars["id"])

	invoice, err := h.service.GetInvoice(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(invoice)
}

func (h *InvoiceHandler) DeleteInvoice(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["id"])
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid Invoice ID"})
		return
	}

	if err := h.service.DeleteInvoice(r.Context(), id); err != nil {
		w.Header().Set("Content-Type", "application/json")
		errMsg := err.Error()
		switch {
		case errMsg == "invoice not found":
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invoice not found"})
		case strings.Contains(strings.ToLower(errMsg), "cannot delete") && strings.Contains(strings.ToLower(errMsg), "paid invoice"):
			w.WriteHeader(http.StatusUnprocessableEntity)
			json.NewEncoder(w).Encode(map[string]string{"error": errMsg})
		default:
			fmt.Printf("[ERROR] Failed to delete invoice %s: %v\n", id, err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Failed to delete invoice. Please try again."})
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Invoice deleted successfully"})
}

func (h *InvoiceHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := uuid.Parse(vars["id"])

	var req struct {
		Status string `json:"status"`
		Notes  string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// In a real app, performedBy would come from auth context
	performedBy := "System User"
	if r.Header.Get("X-User-Name") != "" {
		performedBy = r.Header.Get("X-User-Name")
	}

	err := h.service.UpdateStatus(r.Context(), id, domain.InvoiceStatus(req.Status), req.Notes, performedBy)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Status updated successfully"})
}

func (h *InvoiceHandler) GetAuditLogs(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := uuid.Parse(vars["id"])

	logs, err := h.service.GetAuditLogs(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": logs,
	})
}

// SendInvoice handles sending an invoice (DRAFT → SENT)
func (h *InvoiceHandler) SendInvoice(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["id"])
	if err != nil {
		http.Error(w, "Invalid Invoice ID", http.StatusBadRequest)
		return
	}

	var req dto.SendInvoiceRequest
	// For now, SendInvoiceRequest is empty, but we still decode for future fields
	if r.Body != http.NoBody {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// Ignore decode errors for empty body
		}
	}

	invoice, err := h.service.SendInvoice(r.Context(), id, req)
	if err != nil {
		// Check for specific error types
		if err.Error() == "invoice not found" {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else if strings.HasPrefix(err.Error(), "cannot send") {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(invoice)
}

// DownloadInvoicePDF handles PDF download requests by streaming from S3
func (h *InvoiceHandler) DownloadInvoicePDF(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			fmt.Printf("[ERROR] Panic in DownloadInvoicePDF: %v\n", rec)
			http.Error(w, "internal server error generating invoice pdf", http.StatusInternalServerError)
		}
	}()

	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["id"])
	if err != nil {
		http.Error(w, "Invalid Invoice ID", http.StatusBadRequest)
		return
	}

	reader, contentType, err := h.service.GetInvoicePDFStream(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	defer reader.Close()

	// Serve the PDF file stream
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment; filename=invoice-"+id.String()+".pdf")
	io.Copy(w, reader)
}

// PreviewInvoicePDF handles PDF preview requests (inline display) by streaming from S3
func (h *InvoiceHandler) PreviewInvoicePDF(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			fmt.Printf("[ERROR] Panic in PreviewInvoicePDF: %v\n", rec)
			http.Error(w, "internal server error generating invoice preview", http.StatusInternalServerError)
		}
	}()

	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["id"])
	if err != nil {
		http.Error(w, "Invalid Invoice ID", http.StatusBadRequest)
		return
	}

	reader, contentType, err := h.service.GetInvoicePDFStream(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	defer reader.Close()

	// Serve the PDF file for inline display
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "inline; filename=invoice-"+id.String()+".pdf")
	io.Copy(w, reader)
}
