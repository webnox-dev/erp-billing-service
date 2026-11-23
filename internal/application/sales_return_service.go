package application

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"erp-billing-service/internal/application/dto"
	"erp-billing-service/internal/domain"
	"erp-billing-service/internal/ports/outbound"
	"erp-billing-service/internal/ports/repositories"
	"erp-billing-service/internal/validation"

	shared_events "github.com/efs/shared-events"
	"github.com/google/uuid"
)

// SalesReturnService handles sales return business logic
type SalesReturnService struct {
	salesReturnRepo repositories.SalesReturnRepository
	salesOrderRepo  repositories.SalesOrderRepository
	invoiceRepo     domain.InvoiceRepository
	paymentRepo     domain.PaymentRepository
	rmRepo          domain.ReadModelRepository
	eventPublisher  domain.EventPublisher
	inventoryClient outbound.InventoryClient
}

// NewSalesReturnService creates a new sales return service
func NewSalesReturnService(
	salesReturnRepo repositories.SalesReturnRepository,
	salesOrderRepo repositories.SalesOrderRepository,
	invoiceRepo domain.InvoiceRepository,
	paymentRepo domain.PaymentRepository,
	rmRepo domain.ReadModelRepository,
	eventPublisher domain.EventPublisher,
	inventoryClient outbound.InventoryClient,
) *SalesReturnService {
	return &SalesReturnService{
		salesReturnRepo: salesReturnRepo,
		salesOrderRepo:  salesOrderRepo,
		invoiceRepo:     invoiceRepo,
		paymentRepo:     paymentRepo,
		rmRepo:          rmRepo,
		eventPublisher:  eventPublisher,
		inventoryClient: inventoryClient,
	}
}

// CreateSalesReturn creates and approves a new sales return
func (s *SalesReturnService) CreateSalesReturn(req *dto.CreateSalesReturnRequest) (*dto.SalesReturnResponse, error) {
	// Subscription limit validation
	subClient := validation.NewSubscriptionClient()
	if subClient != nil && req.OrganizationID != uuid.Nil {
		orgIDStr := req.OrganizationID.String()
		allowed, msg, err := subClient.ValidateRestriction(orgIDStr, validation.RestrictionKeyMaxSalesReturns)
		if err != nil {
			log.Printf("[Subscription] Validation error for org %s: %v — allowing (fail-open)", orgIDStr, err)
		} else if !allowed {
			return nil, fmt.Errorf("%s", msg)
		}
	}

	// Retrieve sales order
	salesOrder, err := s.salesOrderRepo.FindByID(req.SalesOrderID)
	if err != nil {
		return nil, fmt.Errorf("failed to find sales order: %w", err)
	}

	// Validate order can be returned (must be delivered)
	if !salesOrder.CanReturn() {
		return nil, fmt.Errorf("sales order must be delivered to create a return (current status: %s)", salesOrder.Status)
	}

	// Create sales return entity
	salesReturn := &domain.SalesReturn{
		ID:             uuid.New(),
		OrganizationID: req.OrganizationID,
		SalesOrderID:   req.SalesOrderID,
		ReturnDate:     req.ReturnDate,
		Status:         domain.SalesReturnStatusDraft,
		ReturnReason:   req.ReturnReason,
		Notes:          req.Notes,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Add return items
	for _, itemDTO := range req.Items {
		item := domain.SalesReturnItem{
			ID:               uuid.New(),
			SalesReturnID:    salesReturn.ID,
			SalesOrderItemID: itemDTO.SalesOrderItemID,
			ReturnedQuantity: itemDTO.ReturnedQuantity,
			UnitPrice:        itemDTO.UnitPrice,
			Tax:              itemDTO.Tax,
			Total:            (itemDTO.ReturnedQuantity * itemDTO.UnitPrice) + itemDTO.Tax,
			Reason:           itemDTO.Reason,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}
		salesReturn.Items = append(salesReturn.Items, item)
	}

	// Calculate totals
	salesReturn.CalculateTotals()

	// Retrieve existing sales returns for this sales order to account for previous non-cancelled returns
	existingReturns, _ := s.salesReturnRepo.FindBySalesOrderID(req.SalesOrderID)
	returnedMap := make(map[uuid.UUID]float64)
	for _, ret := range existingReturns {
		if ret.Status == domain.SalesReturnStatusCancelled {
			continue
		}
		for _, item := range ret.Items {
			returnedMap[item.SalesOrderItemID] += item.ReturnedQuantity
		}
	}

	remainingItems := make([]domain.SalesOrderItem, len(salesOrder.Items))
	for i, item := range salesOrder.Items {
		remainingItems[i] = item
		alreadyReturned := returnedMap[item.ID]
		remainingItems[i].Quantity = item.Quantity - alreadyReturned
	}

	// Validate return quantities against remaining returnable quantities
	if err := salesReturn.ValidateReturnQuantity(remainingItems); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Validate
	if err := salesReturn.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Generate return number
	returnNumber, err := s.salesReturnRepo.GenerateReturnNumber(salesReturn.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate return number: %w", err)
	}
	salesReturn.ReturnNumber = &returnNumber
	if req.SaveAsDraft {
		salesReturn.Status = domain.SalesReturnStatusDraft
	} else {
		salesReturn.Status = domain.SalesReturnStatusApproved
		approvedDate := time.Now()
		salesReturn.ApprovedDate = &approvedDate
	}

	// Save to database
	if err := s.salesReturnRepo.Create(salesReturn); err != nil {
		return nil, fmt.Errorf("failed to create sales return: %w", err)
	}

	// Reload so items carry SalesOrderItem details in response
	if reloaded, rErr := s.salesReturnRepo.FindByID(salesReturn.ID); rErr == nil && reloaded != nil {
		salesReturn = reloaded
	}

	// Publish events
	event := domain.SalesReturnCreatedEvent{
		SalesReturnID: salesReturn.ID.String(),
		ReturnNumber:  *salesReturn.ReturnNumber,
		SalesOrderID:  salesReturn.SalesOrderID.String(),
		ReturnAmount:  salesReturn.ReturnAmount,
	}
	s.eventPublisher.Publish(context.Background(), shared_events.NewEventMetadata(shared_events.EventType("sales_return.created"), shared_events.AggregateType("sales_return"), salesReturn.ID.String()), event)

	return s.toSalesReturnResponse(salesReturn), nil
}

// ApproveReturn transitions a draft sales return request to approved
func (s *SalesReturnService) ApproveReturn(id uuid.UUID) (*dto.SalesReturnResponse, error) {
	salesReturn, err := s.salesReturnRepo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to find sales return: %w", err)
	}
	if !salesReturn.CanApprove() {
		return nil, fmt.Errorf("cannot approve sales return in %s status", salesReturn.Status)
	}
	if err := salesReturn.CanTransitionTo(domain.SalesReturnStatusApproved); err != nil {
		return nil, err
	}
	if salesReturn.ReturnNumber == nil || *salesReturn.ReturnNumber == "" {
		returnNumber, genErr := s.salesReturnRepo.GenerateReturnNumber(salesReturn.OrganizationID)
		if genErr != nil {
			return nil, fmt.Errorf("failed to generate return number: %w", genErr)
		}
		salesReturn.ReturnNumber = &returnNumber
	}
	now := time.Now()
	salesReturn.Status = domain.SalesReturnStatusApproved
	salesReturn.ApprovedDate = &now
	salesReturn.UpdatedAt = now

	if err := s.salesReturnRepo.Update(salesReturn); err != nil {
		return nil, fmt.Errorf("failed to approve sales return: %w", err)
	}
	return s.toSalesReturnResponse(salesReturn), nil
}

// CancelReturn cancels a draft or approved sales return before items are received/refunded
func (s *SalesReturnService) CancelReturn(id uuid.UUID, reason string) (*dto.SalesReturnResponse, error) {
	salesReturn, err := s.salesReturnRepo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to find sales return: %w", err)
	}
	if !salesReturn.CanCancel() {
		return nil, fmt.Errorf("cannot cancel sales return in %s status", salesReturn.Status)
	}
	if err := salesReturn.CanTransitionTo(domain.SalesReturnStatusCancelled); err != nil {
		return nil, err
	}
	now := time.Now()
	salesReturn.Status = domain.SalesReturnStatusCancelled
	salesReturn.CancelledDate = &now
	salesReturn.CancellationReason = strings.TrimSpace(reason)
	salesReturn.UpdatedAt = now

	if err := s.salesReturnRepo.Update(salesReturn); err != nil {
		return nil, fmt.Errorf("failed to cancel sales return: %w", err)
	}
	return s.toSalesReturnResponse(salesReturn), nil
}

func (s *SalesReturnService) restoreStockForReturn(salesReturn *domain.SalesReturn, salesOrder *domain.SalesOrder) error {
	if s.inventoryClient == nil || salesReturn.StockRestored || salesOrder == nil {
		return nil
	}

	goodOrOpenedItems := make([]outbound.StockCheckItem, 0)
	damagedItems := make([]outbound.StockCheckItem, 0)

	orderItemsByID := make(map[uuid.UUID]domain.SalesOrderItem, len(salesOrder.Items))
	for _, soItem := range salesOrder.Items {
		orderItemsByID[soItem.ID] = soItem
	}

	for _, returnItem := range salesReturn.Items {
		soItem, ok := orderItemsByID[returnItem.SalesOrderItemID]
		if !ok || soItem.ItemID == uuid.Nil {
			continue
		}
		itemTypeLower := strings.ToLower(strings.TrimSpace(soItem.ItemType))
		if itemTypeLower == "service" {
			continue
		}
		qty := int32(math.Round(returnItem.ReturnedQuantity))
		if qty <= 0 {
			continue
		}
		entry := outbound.StockCheckItem{
			ItemID:   soItem.ItemID.String(),
			Quantity: qty,
		}
		if strings.EqualFold(strings.TrimSpace(returnItem.Reason), "damaged") {
			damagedItems = append(damagedItems, entry)
		} else {
			goodOrOpenedItems = append(goodOrOpenedItems, entry)
		}
	}

	notesLabel := "Sales Return Received"
	if salesReturn.ReturnNumber != nil && *salesReturn.ReturnNumber != "" {
		notesLabel = fmt.Sprintf("Sales Return %s Received", *salesReturn.ReturnNumber)
	}

	if len(goodOrOpenedItems) > 0 {
		if err := s.inventoryClient.UpdateStock(context.Background(), goodOrOpenedItems, "return", "sales_return", salesReturn.ID.String(), notesLabel); err != nil {
			return fmt.Errorf("failed to restore sellable stock for sales return: %w", err)
		}
	}
	if len(damagedItems) > 0 {
		if err := s.inventoryClient.UpdateStock(context.Background(), damagedItems, "damaged_return", "sales_return", salesReturn.ID.String(), notesLabel+" (Damaged)"); err != nil {
			return fmt.Errorf("failed to record damaged stock for sales return: %w", err)
		}
	}

	return nil
}

// ReceiveReturn marks a sales return as received and updates inventory stock
func (s *SalesReturnService) ReceiveReturn(id uuid.UUID, req *dto.ReceiveReturnRequest) (*dto.SalesReturnResponse, error) {
	// Retrieve return
	salesReturn, err := s.salesReturnRepo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to find sales return: %w", err)
	}

	// Check if can receive
	if !salesReturn.CanReceive() {
		return nil, fmt.Errorf("cannot receive sales return in %s status", salesReturn.Status)
	}

	// Update status
	if err := salesReturn.CanTransitionTo(domain.SalesReturnStatusReceived); err != nil {
		return nil, err
	}

	// Retrieve sales order to resolve item IDs and item types for stock update
	salesOrder, soErr := s.salesOrderRepo.FindByID(salesReturn.SalesOrderID)
	if soErr == nil && salesOrder != nil {
		if stockErr := s.restoreStockForReturn(salesReturn, salesOrder); stockErr != nil {
			return nil, stockErr
		}
		salesReturn.StockRestored = true
	}

	salesReturn.Status = domain.SalesReturnStatusReceived
	salesReturn.ReceivedDate = &req.ReceivedDate
	salesReturn.ReceivingNotes = req.ReceivingNotes
	salesReturn.UpdatedAt = time.Now()

	// Save
	if err := s.salesReturnRepo.Update(salesReturn); err != nil {
		return nil, fmt.Errorf("failed to receive sales return: %w", err)
	}

	// Publish event
	event := domain.SalesReturnReceivedEvent{
		SalesReturnID: salesReturn.ID.String(),
		ReceivedDate:  req.ReceivedDate,
	}
	s.eventPublisher.Publish(context.Background(), shared_events.NewEventMetadata(shared_events.EventType("sales_return.received"), shared_events.AggregateType("sales_return"), salesReturn.ID.String()), event)

	return s.toSalesReturnResponse(salesReturn), nil
}

// ProcessRefund processes a refund payment for a sales return
func (s *SalesReturnService) ProcessRefund(id uuid.UUID, req *dto.ProcessRefundRequest) (*dto.SalesReturnResponse, error) {
	fmt.Printf("[DEBUG] ProcessRefund called for sales return ID: %s\n", id)

	// Retrieve return
	salesReturn, err := s.salesReturnRepo.FindByID(id)
	if err != nil {
		fmt.Printf("[ERROR] Failed to find sales return: %v\n", err)
		return nil, fmt.Errorf("failed to find sales return: %w", err)
	}
	fmt.Printf("[DEBUG] Sales return found: status=%s, refund_payment_id=%v\n", salesReturn.Status, salesReturn.RefundPaymentID)

	// Check if can refund
	if !salesReturn.CanRefund() {
		fmt.Printf("[ERROR] Cannot refund: status=%s, refund_payment_id=%v\n", salesReturn.Status, salesReturn.RefundPaymentID)
		return nil, fmt.Errorf("cannot process refund for sales return in %s status or refund already processed", salesReturn.Status)
	}

	// Retrieve sales order to get items and fallback invoice link
	salesOrder, err := s.salesOrderRepo.FindByID(salesReturn.SalesOrderID)
	if err != nil {
		fmt.Printf("[ERROR] Failed to find sales order: %v\n", err)
		return nil, fmt.Errorf("failed to find sales order: %w", err)
	}
	fmt.Printf("[DEBUG] Sales order found: id=%s\n", salesOrder.ID)

	// Find an eligible paid invoice for this sales order
	invoices, err := s.invoiceRepo.ListBySalesOrderID(context.Background(), salesReturn.SalesOrderID)
	if err != nil {
		return nil, fmt.Errorf("failed to look up invoices for sales order: %w", err)
	}

	const epsilon = 0.01
	var invoice *domain.Invoice
	for i := range invoices {
		if !invoices[i].IsPartialInvoice && invoices[i].CanRefund() && invoices[i].PaidAmount+epsilon >= salesReturn.ReturnAmount {
			invoice = &invoices[i]
			break
		}
	}

	if invoice == nil {
		// Friendly fallback: try the sales order's direct invoice_id pointer if set
		if salesOrder.InvoiceID != nil {
			fallback, ferr := s.invoiceRepo.GetByID(context.Background(), *salesOrder.InvoiceID)
			if ferr == nil && fallback != nil && fallback.CanRefund() && fallback.PaidAmount+epsilon >= salesReturn.ReturnAmount {
				invoice = fallback
			}
		}
	}

	if invoice == nil {
		if len(invoices) == 0 && salesOrder.InvoiceID == nil {
			return nil, fmt.Errorf("no invoices found for this sales order — cannot process refund")
		}
		return nil, fmt.Errorf(
			"no eligible paid invoice found with sufficient refundable amount (%.2f) for this sales order — payment must be recorded before a refund can be processed",
			salesReturn.ReturnAmount,
		)
	}

	fmt.Printf("[DEBUG] Found paid invoice for refund: id=%s, status=%s, paid=%.2f\n", invoice.ID, invoice.Status, invoice.PaidAmount)

	// Create refund payment (negative amount)
	refundPayment := &domain.Payment{
		ID:             uuid.New(),
		OrganizationID: salesReturn.OrganizationID,
		InvoiceID:      invoice.ID,
		Amount:         -salesReturn.ReturnAmount, // Negative for refund
		PaymentDate:    req.PaymentDate,
		Method:         domain.PaymentMethod(req.Method),
		Reference:      req.Reference,
		Status:         domain.PaymentStatusCompleted,
		PaymentType:    domain.PaymentTypeRefund,
		SalesReturnID:  &salesReturn.ID,
		Notes:          req.Notes,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Save payment
	if err := s.paymentRepo.Create(context.Background(), refundPayment); err != nil {
		return nil, fmt.Errorf("failed to create refund payment: %w", err)
	}

	// Update invoice refunded & net paid revenue figures without inflating unpaid balance
	invoice.RefundedAmount = roundCurrency(invoice.RefundedAmount + salesReturn.ReturnAmount)
	invoice.PaidAmount = math.Max(0, roundCurrency(invoice.PaidAmount-salesReturn.ReturnAmount))
	invoice.PDFPath = nil
	invoice.UpdatedAt = time.Now()
	if err := s.invoiceRepo.Update(context.Background(), invoice); err != nil {
		return nil, fmt.Errorf("failed to update invoice: %w", err)
	}

	// Restore stock only if not already restored on ReceiveReturn
	if !salesReturn.StockRestored {
		if stockErr := s.restoreStockForReturn(salesReturn, salesOrder); stockErr != nil {
			fmt.Printf("sales return refunded but failed to update stock: %v\n", stockErr)
		} else {
			salesReturn.StockRestored = true
		}
	}

	// Update sales return
	if err := salesReturn.CanTransitionTo(domain.SalesReturnStatusRefunded); err != nil {
		return nil, err
	}
	salesReturn.Status = domain.SalesReturnStatusRefunded
	salesReturn.RefundPaymentID = &refundPayment.ID
	refundedDate := time.Now()
	salesReturn.RefundedDate = &refundedDate
	salesReturn.UpdatedAt = time.Now()

	// Save
	if err := s.salesReturnRepo.Update(salesReturn); err != nil {
		return nil, fmt.Errorf("failed to update sales return: %w", err)
	}

	// Publish events
	paymentEvent := domain.PaymentRecordedEvent{
		PaymentID:   refundPayment.ID.String(),
		InvoiceID:   invoice.ID.String(),
		Amount:      refundPayment.Amount,
		PaymentType: string(refundPayment.PaymentType),
	}
	s.eventPublisher.Publish(context.Background(), shared_events.NewEventMetadata(shared_events.EventType("billing.payment.refund.recorded"), shared_events.AggregateType("payment"), refundPayment.ID.String()), paymentEvent)

	returnEvent := domain.SalesReturnRefundedEvent{
		SalesReturnID: salesReturn.ID.String(),
		RefundAmount:  salesReturn.ReturnAmount,
		PaymentID:     refundPayment.ID.String(),
	}
	s.eventPublisher.Publish(context.Background(), shared_events.NewEventMetadata(shared_events.EventType("sales_return.refunded"), shared_events.AggregateType("sales_return"), salesReturn.ID.String()), returnEvent)

	return s.toSalesReturnResponse(salesReturn), nil
}

// GetSalesReturn retrieves a sales return by ID
func (s *SalesReturnService) GetSalesReturn(id uuid.UUID) (*dto.SalesReturnResponse, error) {
	salesReturn, err := s.salesReturnRepo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to find sales return: %w", err)
	}
	return s.toSalesReturnResponse(salesReturn), nil
}

// ListSalesReturns retrieves sales returns with filters
func (s *SalesReturnService) ListSalesReturns(orgID uuid.UUID, filters *dto.SalesReturnFilters) ([]*dto.SalesReturnResponse, int64, error) {
	salesReturns, total, err := s.salesReturnRepo.List(orgID, filters)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list sales returns: %w", err)
	}

	responses := make([]*dto.SalesReturnResponse, len(salesReturns))
	for i, salesReturn := range salesReturns {
		responses[i] = s.toSalesReturnResponse(salesReturn)
	}

	return responses, total, nil
}

// GetReturnsBySalesOrder retrieves all returns for a sales order
func (s *SalesReturnService) GetReturnsBySalesOrder(orderID uuid.UUID) ([]*dto.SalesReturnResponse, error) {
	salesReturns, err := s.salesReturnRepo.FindBySalesOrderID(orderID)
	if err != nil {
		return nil, fmt.Errorf("failed to find sales returns: %w", err)
	}

	responses := make([]*dto.SalesReturnResponse, len(salesReturns))
	for i, salesReturn := range salesReturns {
		responses[i] = s.toSalesReturnResponse(salesReturn)
	}

	return responses, nil
}

// toSalesReturnResponse converts domain entity to DTO
func (s *SalesReturnService) toSalesReturnResponse(salesReturn *domain.SalesReturn) *dto.SalesReturnResponse {
	ctxWithOrg := withOrgInContext(context.Background(), salesReturn.OrganizationID)

	items := make([]dto.SalesReturnItemDTO, len(salesReturn.Items))
	for i, item := range salesReturn.Items {
		itemDTO := dto.SalesReturnItemDTO{
			ID:               &item.ID,
			SalesOrderItemID: item.SalesOrderItemID,
			ReturnedQuantity: item.ReturnedQuantity,
			UnitPrice:        item.UnitPrice,
			Tax:              item.Tax,
			Total:            item.Total,
			Reason:           item.Reason,
			Condition:        item.Reason,
		}
		if item.SalesOrderItem != nil {
			itemID := item.SalesOrderItem.ItemID
			itemDTO.ItemID = &itemID
			itemDTO.Name = item.SalesOrderItem.Name
			itemDTO.Description = item.SalesOrderItem.Description
		}
		items[i] = itemDTO
	}

	var orderNumber *string
	var customerID *uuid.UUID
	var customerName string
	so := salesReturn.SalesOrder
	if so == nil && s.salesOrderRepo != nil {
		if fetched, err := s.salesOrderRepo.FindByID(salesReturn.SalesOrderID); err == nil {
			so = fetched
		}
	}
	if so != nil {
		orderNumber = so.OrderNumber
		cid := so.CustomerID
		customerID = &cid
		if s.rmRepo != nil {
			if cust, err := s.rmRepo.GetCustomer(ctxWithOrg, so.CustomerID); err == nil && cust != nil {
				customerName = cust.DisplayName
				if customerName == "" {
					customerName = strings.TrimSpace(cust.FirstName + " " + cust.LastName)
				}
				if customerName == "" {
					customerName = cust.CompanyName
				}
			}
		}
	}

	currency := ""
	if s.rmRepo != nil {
		if org, err := s.rmRepo.GetOrganization(ctxWithOrg, salesReturn.OrganizationID); err == nil && org != nil {
			currency = org.Currency
		}
	}

	return &dto.SalesReturnResponse{
		ID:                 salesReturn.ID,
		OrganizationID:     salesReturn.OrganizationID,
		SalesOrderID:       salesReturn.SalesOrderID,
		SalesOrderNumber:   orderNumber,
		CustomerID:         customerID,
		CustomerName:       customerName,
		Currency:           currency,
		ReturnNumber:       salesReturn.ReturnNumber,
		ReturnDate:         ConvertToOrgTZValue(ctxWithOrg, salesReturn.ReturnDate, salesReturn.OrganizationID, s.rmRepo),
		Status:             string(salesReturn.Status),
		ReturnAmount:       salesReturn.ReturnAmount,
		ReturnReason:       salesReturn.ReturnReason,
		Notes:              salesReturn.Notes,
		ApprovedDate:       ConvertToOrgTZ(ctxWithOrg, salesReturn.ApprovedDate, salesReturn.OrganizationID, s.rmRepo),
		ReceivedDate:       ConvertToOrgTZ(ctxWithOrg, salesReturn.ReceivedDate, salesReturn.OrganizationID, s.rmRepo),
		ReceivingNotes:     salesReturn.ReceivingNotes,
		RefundedDate:       ConvertToOrgTZ(ctxWithOrg, salesReturn.RefundedDate, salesReturn.OrganizationID, s.rmRepo),
		RefundPaymentID:    salesReturn.RefundPaymentID,
		CancelledDate:      ConvertToOrgTZ(ctxWithOrg, salesReturn.CancelledDate, salesReturn.OrganizationID, s.rmRepo),
		CancellationReason: salesReturn.CancellationReason,
		StockRestored:      salesReturn.StockRestored,
		Items:              items,
		CreatedAt:          ConvertToOrgTZValue(ctxWithOrg, salesReturn.CreatedAt, salesReturn.OrganizationID, s.rmRepo),
		UpdatedAt:          ConvertToOrgTZValue(ctxWithOrg, salesReturn.UpdatedAt, salesReturn.OrganizationID, s.rmRepo),
	}
}
