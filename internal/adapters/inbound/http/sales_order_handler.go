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

// SalesOrderHandler handles HTTP requests for sales orders
type SalesOrderHandler struct {
	service *application.SalesOrderService
}

// NewSalesOrderHandler creates a new sales order handler
func NewSalesOrderHandler(service *application.SalesOrderService) *SalesOrderHandler {
	return &SalesOrderHandler{service: service}
}

// CreateSalesOrder handles POST /api/v1/sales-orders
func (h *SalesOrderHandler) CreateSalesOrder(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateSalesOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body: " + err.Error()})
		return
	}

	// Get organization ID from header
	orgIDStr := r.Header.Get("X-Organization-ID")
	orgID, _ := uuid.Parse(orgIDStr)
	req.OrganizationID = orgID

	order, err := h.service.CreateSalesOrder(&req)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		errStr := err.Error()
		if strings.Contains(errStr, "limit reached") || strings.Contains(errStr, "upgrade your subscription") || strings.HasPrefix(errStr, "plan limit reached") {
			w.WriteHeader(http.StatusForbidden)
		} else if strings.Contains(errStr, "validation failed") || strings.Contains(errStr, "stock unavailable") {
			w.WriteHeader(http.StatusBadRequest)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		json.NewEncoder(w).Encode(map[string]string{"error": errStr})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(order)
}

// UpdateSalesOrder handles PUT /api/v1/sales-orders/:id
func (h *SalesOrderHandler) UpdateSalesOrder(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["id"])
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid sales order ID"})
		return
	}

	var req dto.UpdateSalesOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	order, err := h.service.UpdateSalesOrder(id, &req)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(err.Error(), "validation failed") || strings.Contains(err.Error(), "cannot edit") {
			w.WriteHeader(http.StatusBadRequest)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(order)
}

// GetSalesOrder handles GET /api/v1/sales-orders/:id
func (h *SalesOrderHandler) GetSalesOrder(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["id"])
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid sales order ID"})
		return
	}

	order, err := h.service.GetSalesOrder(id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(order)
}

// ListSalesOrders handles GET /api/v1/sales-orders
func (h *SalesOrderHandler) ListSalesOrders(w http.ResponseWriter, r *http.Request) {
	orgIDStr := r.Header.Get("X-Organization-ID")
	orgID, _ := uuid.Parse(orgIDStr)

	// Parse query parameters
	filters := &dto.SalesOrderFilters{
		Page:     1,
		PageSize: 500,
	}

	if page := r.URL.Query().Get("page"); page != "" {
		if p, err := strconv.Atoi(page); err == nil {
			filters.Page = p
		}
	}

	if pageSize := r.URL.Query().Get("page_size"); pageSize != "" {
		if ps, err := strconv.Atoi(pageSize); err == nil {
			filters.PageSize = ps
		}
	}

	if status := r.URL.Query().Get("status"); status != "" {
		filters.Status = &status
	}

	if customerID := r.URL.Query().Get("customer_id"); customerID != "" {
		if cid, err := uuid.Parse(customerID); err == nil {
			filters.CustomerID = &cid
		}
	}

	if customerName := r.URL.Query().Get("customer_name"); customerName != "" {
		filters.CustomerName = &customerName
	}

	if search := r.URL.Query().Get("search"); search != "" {
		filters.Search = &search
	}

	if itemName := r.URL.Query().Get("item_name"); itemName != "" {
		filters.ItemName = &itemName
	}

	// Amount range filters
	if minAmountStr := r.URL.Query().Get("min_amount"); minAmountStr != "" {
		if val, err := strconv.ParseFloat(minAmountStr, 64); err == nil {
			filters.MinAmount = &val
		}
	}
	if maxAmountStr := r.URL.Query().Get("max_amount"); maxAmountStr != "" {
		if val, err := strconv.ParseFloat(maxAmountStr, 64); err == nil {
			filters.MaxAmount = &val
		}
	}

	dateType := r.URL.Query().Get("date_type")

	// Order date filters
	if orderStart := r.URL.Query().Get("order_date_start"); orderStart != "" {
		if t, err := time.Parse("2006-01-02", orderStart); err == nil {
			filters.OrderDateStart = &t
		} else if t, err := time.Parse(time.RFC3339, orderStart); err == nil {
			filters.OrderDateStart = &t
		}
	} else if dateType == "order_date" {
		if startDate := r.URL.Query().Get("start_date"); startDate != "" {
			if t, err := time.Parse("2006-01-02", startDate); err == nil {
				filters.OrderDateStart = &t
			} else if t, err := time.Parse(time.RFC3339, startDate); err == nil {
				filters.OrderDateStart = &t
			}
		}
	}

	if orderEnd := r.URL.Query().Get("order_date_end"); orderEnd != "" {
		if t, err := time.Parse("2006-01-02", orderEnd); err == nil {
			endOfDay := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, time.UTC)
			filters.OrderDateEnd = &endOfDay
		} else if t, err := time.Parse(time.RFC3339, orderEnd); err == nil {
			filters.OrderDateEnd = &t
		}
	} else if dateType == "order_date" {
		if endDate := r.URL.Query().Get("end_date"); endDate != "" {
			if t, err := time.Parse("2006-01-02", endDate); err == nil {
				endOfDay := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, time.UTC)
				filters.OrderDateEnd = &endOfDay
			} else if t, err := time.Parse(time.RFC3339, endDate); err == nil {
				filters.OrderDateEnd = &t
			}
		}
	}

	// Due date filters (Expected Shipment Date)
	if dueStart := r.URL.Query().Get("due_date_start"); dueStart != "" {
		if t, err := time.Parse("2006-01-02", dueStart); err == nil {
			filters.DueDateStart = &t
		} else if t, err := time.Parse(time.RFC3339, dueStart); err == nil {
			filters.DueDateStart = &t
		}
	} else if dateType == "due_date" {
		if startDate := r.URL.Query().Get("start_date"); startDate != "" {
			if t, err := time.Parse("2006-01-02", startDate); err == nil {
				filters.DueDateStart = &t
			} else if t, err := time.Parse(time.RFC3339, startDate); err == nil {
				filters.DueDateStart = &t
			}
		}
	}

	if dueEnd := r.URL.Query().Get("due_date_end"); dueEnd != "" {
		if t, err := time.Parse("2006-01-02", dueEnd); err == nil {
			endOfDay := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, time.UTC)
			filters.DueDateEnd = &endOfDay
		} else if t, err := time.Parse(time.RFC3339, dueEnd); err == nil {
			filters.DueDateEnd = &t
		}
	} else if dateType == "due_date" {
		if endDate := r.URL.Query().Get("end_date"); endDate != "" {
			if t, err := time.Parse("2006-01-02", endDate); err == nil {
				endOfDay := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, time.UTC)
				filters.DueDateEnd = &endOfDay
			} else if t, err := time.Parse(time.RFC3339, endDate); err == nil {
				filters.DueDateEnd = &t
			}
		}
	}

	// Created at date filters
	if createdAtStart := r.URL.Query().Get("created_at_start"); createdAtStart != "" {
		if t, err := time.Parse("2006-01-02", createdAtStart); err == nil {
			filters.FromDate = &t
		} else if t, err := time.Parse(time.RFC3339, createdAtStart); err == nil {
			filters.FromDate = &t
		}
	} else if dateType == "created_at" {
		if startDate := r.URL.Query().Get("start_date"); startDate != "" {
			if t, err := time.Parse("2006-01-02", startDate); err == nil {
				filters.FromDate = &t
			} else if t, err := time.Parse(time.RFC3339, startDate); err == nil {
				filters.FromDate = &t
			}
		}
	} else if fromDate := r.URL.Query().Get("from_date"); fromDate != "" {
		if t, err := time.Parse("2006-01-02", fromDate); err == nil {
			filters.FromDate = &t
		} else if t, err := time.Parse(time.RFC3339, fromDate); err == nil {
			filters.FromDate = &t
		}
	}

	if createdAtEnd := r.URL.Query().Get("created_at_end"); createdAtEnd != "" {
		if t, err := time.Parse("2006-01-02", createdAtEnd); err == nil {
			endOfDay := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, time.UTC)
			filters.ToDate = &endOfDay
		} else if t, err := time.Parse(time.RFC3339, createdAtEnd); err == nil {
			filters.ToDate = &t
		}
	} else if dateType == "created_at" {
		if endDate := r.URL.Query().Get("end_date"); endDate != "" {
			if t, err := time.Parse("2006-01-02", endDate); err == nil {
				endOfDay := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, time.UTC)
				filters.ToDate = &endOfDay
			} else if t, err := time.Parse(time.RFC3339, endDate); err == nil {
				filters.ToDate = &t
			}
		}
	} else if toDate := r.URL.Query().Get("to_date"); toDate != "" {
		if t, err := time.Parse("2006-01-02", toDate); err == nil {
			endOfDay := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, time.UTC)
			filters.ToDate = &endOfDay
		} else if t, err := time.Parse(time.RFC3339, toDate); err == nil {
			filters.ToDate = &t
		}
	}

	orders, total, err := h.service.ListSalesOrders(orgID, filters)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":      orders,
		"total":     total,
		"page":      filters.Page,
		"page_size": filters.PageSize,
	})
}

// ConfirmSalesOrder handles POST /api/v1/sales-orders/:id/confirm
func (h *SalesOrderHandler) ConfirmSalesOrder(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["id"])
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid sales order ID"})
		return
	}

	order, err := h.service.ConfirmSalesOrder(id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(order)
}

// CreateInvoiceFromOrder handles POST /api/v1/sales-orders/:id/create-invoice
func (h *SalesOrderHandler) CreateInvoiceFromOrder(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["id"])
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid sales order ID"})
		return
	}

	invoice, err := h.service.CreateInvoiceFromOrder(id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(invoice)
}

// MarkAsShipped handles POST /api/v1/sales-orders/:id/ship
func (h *SalesOrderHandler) MarkAsShipped(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["id"])
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid sales order ID"})
		return
	}

	var req dto.MarkAsShippedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	order, err := h.service.MarkAsShipped(id, &req)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(order)
}

// MarkAsDelivered handles POST /api/v1/sales-orders/:id/deliver
func (h *SalesOrderHandler) MarkAsDelivered(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["id"])
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid sales order ID"})
		return
	}

	var req dto.MarkAsDeliveredRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	order, err := h.service.MarkAsDelivered(id, &req)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		errMsg := err.Error()
		if isDBConnError(errMsg) {
			// Infrastructure / connectivity error – hide internal details from the client
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "A temporary database error occurred. Please try again in a moment."})
		} else if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "no rows") {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Sales order not found"})
		} else {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": errMsg})
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(order)
}

// DeleteSalesOrder handles DELETE /api/v1/sales-orders/:id
func (h *SalesOrderHandler) DeleteSalesOrder(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["id"])
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid sales order ID"})
		return
	}

	if err := h.service.DeleteSalesOrder(id); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Sales order deleted successfully"})
}

// CancelSalesOrder handles DELETE /api/v1/sales-orders/:id/cancel
func (h *SalesOrderHandler) CancelSalesOrder(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["id"])
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid sales order ID"})
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// If no body, use default reason
		req.Reason = "Cancelled by user"
	}

	if err := h.service.CancelSalesOrder(id, req.Reason); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Sales order cancelled successfully"})
}

// SendSalesOrder handles POST /billing/sales-orders/{id}/send
func (h *SalesOrderHandler) SendSalesOrder(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["id"])
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid sales order ID"})
		return
	}

	order, err := h.service.SendSalesOrder(r.Context(), id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(order)
}

// isDBConnError returns true when the error string indicates a database
// connectivity or infrastructure problem (e.g. admin-terminated connection,
// network reset, connection pool exhaustion). These should never be surfaced
// verbatim to API clients.
func isDBConnError(errMsg string) bool {
	connPatterns := []string{
		"57P01",                           // SQLSTATE: admin termination
		"57P02",                           // SQLSTATE: crash shutdown
		"57P03",                           // SQLSTATE: cannot connect now
		"terminating connection",          // postgres admin command
		"connection refused",              // TCP-level refusal
		"connection reset by peer",        // TCP reset
		"broken pipe",                     // write on closed connection
		"EOF",                             // unexpected EOF from DB
		"connection timed out",            // dial timeout
		"no connection to the server",     // libpq message
		"failed to connect",               // generic pool failure
		"connection pool",                 // pool exhaustion
		"pgconn",                          // pgx connection-level errors
		"FATAL",                           // PostgreSQL FATAL severity
	}
	for _, pattern := range connPatterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}
	return false
}
