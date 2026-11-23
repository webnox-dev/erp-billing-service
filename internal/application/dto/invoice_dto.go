package dto

import (
	"time"

	"github.com/google/uuid"
)

type CreateInvoiceRequest struct {
	// Source tracking - makes billing source-agnostic
	SourceSystem      string  `json:"source_system"`       // FSM, CRM, INVENTORY, MANUAL
	SourceReferenceID *string `json:"source_reference_id"` // e.g., "WO-12345", "DEAL-789"

	Subject           string              `json:"subject" validate:"required"`
	CustomerID        uuid.UUID           `json:"customer_id" validate:"required"`
	ContactID         *uuid.UUID          `json:"contact_id"`
	OwnerID           *uuid.UUID          `json:"owner_id"`
	ServiceAddressID  *uuid.UUID          `json:"service_address_id,omitempty"`
	BillingAddressID  *uuid.UUID          `json:"billing_address_id,omitempty"`
	ShippingAddressID *uuid.UUID          `json:"shipping_address_id,omitempty"`
	InvoiceDate     time.Time           `json:"invoice_date"`
	DueDate         time.Time           `json:"due_date"`
	ReferenceNo     string              `json:"reference_no"`
	PaymentTerms    string              `json:"payment_terms"`
	SalesOrder      string              `json:"sales_order"`
	SalesOrderID      *uuid.UUID          `json:"sales_order_id"`
	PurchaseOrder     string              `json:"purchase_order"`
	ServiceCategoryID *uuid.UUID          `json:"service_category_id,omitempty"`
	PartCategoryID    *uuid.UUID          `json:"part_category_id,omitempty"`
	Currency        string              `json:"currency"`
	Adjustment      float64             `json:"adjustment"`
	ExciseDuty      float64             `json:"excise_duty"`
	SalesCommission float64             `json:"sales_commission"`
	Terms           string              `json:"terms"`
	Notes           string              `json:"notes"`
	BillingStreet   string              `json:"billing_street"`
	BillingCity     string              `json:"billing_city"`
	BillingState    string              `json:"billing_state"`
	BillingCode     string              `json:"billing_code"`
	BillingCountry  string              `json:"billing_country"`
	ShippingStreet  string              `json:"shipping_street"`
	ShippingCity    string              `json:"shipping_city"`
	ShippingState   string              `json:"shipping_state"`
	ShippingCode    string              `json:"shipping_code"`
	ShippingCountry string              `json:"shipping_country"`
	Items           []CreateInvoiceItem `json:"items" validate:"required,min=1"`
}

type CreateInvoiceItem struct {
	ItemID      uuid.UUID `json:"item_id" validate:"required"`
	ItemType    string    `json:"item_type"` // Optional, defaults to service if not determined
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Quantity    float64   `json:"quantity" validate:"required,gt=0"`
	UnitPrice   float64   `json:"unit_price" validate:"required,gte=0"`
	Discount    float64    `json:"discount"`
	Tax         float64    `json:"tax"`
	ServiceCategoryID *uuid.UUID `json:"service_category_id,omitempty"`
	PartCategoryID    *uuid.UUID `json:"part_category_id,omitempty"`
	// Module-specific metadata (FSM: technician_id, CRM: deal_id, etc.)
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

type InvoiceResponse struct {
	ID            uuid.UUID `json:"id"`
	InvoiceNumber *string   `json:"invoice_number"` // Nullable - only set when SENT

	// Source tracking
	SourceSystem      string  `json:"source_system"`
	SourceReferenceID *string `json:"source_reference_id,omitempty"`

	Subject         string     `json:"subject"`
	Status          string     `json:"status"`
	SubTotal        float64    `json:"sub_total"`
	DiscountTotal   float64    `json:"discount_total"`
	TaxTotal        float64    `json:"tax_total"`
	TotalAmount     float64    `json:"total_amount"`
	PaidAmount      float64    `json:"paid_amount"`
	RefundedAmount  float64    `json:"refunded_amount"`
	BalanceAmount   float64    `json:"balance_amount"`
	Adjustment      float64    `json:"adjustment"`
	ExciseDuty      float64    `json:"excise_duty"`
	SalesCommission float64    `json:"sales_commission"`
	SalesOrder      string     `json:"sales_order"`
	SalesOrderID    *uuid.UUID `json:"sales_order_id,omitempty"`
	PurchaseOrder   string     `json:"purchase_order"`
	PaymentTerms    string     `json:"payment_terms"`
	Currency        string     `json:"currency"`
	ServiceCategoryID *uuid.UUID `json:"service_category_id,omitempty"`
	PartCategoryID    *uuid.UUID `json:"part_category_id,omitempty"`
	ParentInvoiceID     *uuid.UUID               `json:"parent_invoice_id,omitempty"`
	ParentInvoiceNumber *string                  `json:"parent_invoice_number,omitempty"`
	IsPartialInvoice    bool                     `json:"is_partial_invoice"`
	ActualAmount        float64                  `json:"actual_amount,omitempty"`
	PreviousPaidAmount  float64                  `json:"previous_paid_amount,omitempty"`
	ActualDiscountTotal float64                  `json:"actual_discount_total,omitempty"`
	ActualTaxTotal      float64                  `json:"actual_tax_total,omitempty"`
	ActualAdjustment    float64                  `json:"actual_adjustment,omitempty"`
	OrderBalanceAmount  float64                  `json:"order_balance_amount,omitempty"`
	PreviousPayments    []PaymentResponse        `json:"previous_payments,omitempty"`
	ChildInvoices       []InvoiceSummaryResponse `json:"child_invoices,omitempty"`
	OwnerID         *uuid.UUID `json:"owner_id"`
	CustomerID         uuid.UUID  `json:"customer_id"`
	ContactID          *uuid.UUID `json:"contact_id"`
	ServiceAddressID   *uuid.UUID `json:"service_address_id,omitempty"`
	BillingAddressID   *uuid.UUID `json:"billing_address_id,omitempty"`
	ShippingAddressID  *uuid.UUID `json:"shipping_address_id,omitempty"`
	InvoiceDate       time.Time  `json:"invoice_date"`
	DueDate         time.Time  `json:"due_date"`

	// PDF path - populated when invoice is sent
	PDFPath *string `json:"pdf_path,omitempty"`

	Customer        *CustomerResponse     `json:"customer,omitempty"`
	Contact         *ContactResponse      `json:"contact,omitempty"`
	Organization    *OrganizationResponse `json:"organization,omitempty"`
	AuthorName      string                `json:"author_name,omitempty"`
	Items           []ItemResponse        `json:"items,omitempty"`
	Payments        []PaymentResponse     `json:"payments,omitempty"`
	Notes           string            `json:"notes"`
	Terms           string            `json:"terms"`
	BillingStreet   string            `json:"billing_street"`
	BillingCity     string            `json:"billing_city"`
	BillingState    string            `json:"billing_state"`
	BillingCode     string            `json:"billing_code"`
	BillingCountry  string            `json:"billing_country"`
	ShippingStreet  string            `json:"shipping_street"`
	ShippingCity    string            `json:"shipping_city"`
	ShippingState   string            `json:"shipping_state"`
	ShippingCode    string            `json:"shipping_code"`
	ShippingCountry string            `json:"shipping_country"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CustomerResponse struct {
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
	CompanyName string    `json:"company_name"`
	Email       string    `json:"email,omitempty"`
	Phone       string    `json:"phone,omitempty"`
	TaxNumber   string    `json:"tax_number,omitempty"`
}

type ContactResponse struct {
	ID        uuid.UUID `json:"id"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone,omitempty"`
}

type OrganizationResponse struct {
	ID               uuid.UUID `json:"id"`
	OrganizationName string    `json:"organization_name"`
	Address          string    `json:"address,omitempty"`
	City             string    `json:"city,omitempty"`
	State            string    `json:"state,omitempty"`
	ZipCode          string    `json:"zip_code,omitempty"`
	Country          string    `json:"country,omitempty"`
	Phone            string    `json:"phone,omitempty"`
	Email            string    `json:"email,omitempty"`
	Website          string    `json:"website,omitempty"`
	GSTIN            string    `json:"gstin,omitempty"`
	IconURL          string    `json:"icon_url,omitempty"`
}

type InvoiceSummaryResponse struct {
	ID                  uuid.UUID  `json:"id"`
	InvoiceNumber       *string    `json:"invoice_number,omitempty"`
	Status              string     `json:"status"`
	IsPartialInvoice    bool       `json:"is_partial_invoice"`
	ParentInvoiceID     *uuid.UUID `json:"parent_invoice_id,omitempty"`
	ParentInvoiceNumber *string    `json:"parent_invoice_number,omitempty"`
	ActualAmount        float64    `json:"actual_amount,omitempty"`
	PreviousPaidAmount  float64    `json:"previous_paid_amount,omitempty"`
	SubTotal            float64    `json:"sub_total"`
	DiscountTotal       float64    `json:"discount_total"`
	TaxTotal            float64    `json:"tax_total"`
	Adjustment          float64    `json:"adjustment"`
	TotalAmount         float64    `json:"total_amount"`
	PaidAmount          float64    `json:"paid_amount"`
	BalanceAmount       float64    `json:"balance_amount"`
	OrderBalanceAmount  float64    `json:"order_balance_amount,omitempty"`
	Method              string     `json:"method,omitempty"`
	TransactionID       string     `json:"transaction_id,omitempty"`
	InvoiceDate         time.Time  `json:"invoice_date"`
	CreatedAt           time.Time  `json:"created_at"`
}

type ItemResponse struct {
	ItemID      uuid.UUID `json:"item_id"`
	ItemType    string    `json:"item_type"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Quantity    float64   `json:"quantity"`
	UnitPrice   float64   `json:"unit_price"`
	Discount    float64   `json:"discount"`
	Tax         float64    `json:"tax"`
	Total       float64    `json:"total"`
	ServiceCategoryID *uuid.UUID `json:"service_category_id,omitempty"`
	PartCategoryID    *uuid.UUID `json:"part_category_id,omitempty"`
	// Module-specific metadata returned as-is
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// SendInvoiceRequest represents the request to send an invoice
// Reserved for future fields like email recipients, notification preferences, etc.
type SendInvoiceRequest struct {
	// Future: EmailRecipients []string `json:"email_recipients"`
	// Future: SendEmail bool `json:"send_email"`
}

type InvoiceAuditLogResponse struct {
	ID          uuid.UUID `json:"id"`
	InvoiceID   uuid.UUID `json:"invoice_id"`
	Action      string    `json:"action"`
	OldStatus   string    `json:"old_status"`
	NewStatus   string    `json:"new_status"`
	Notes       string    `json:"notes"`
	PerformedBy string    `json:"performed_by"`
	CreatedAt   time.Time `json:"created_at"`
}

type InvoiceFilterParams struct {
	CustomerID     *uuid.UUID
	CategoryID     *uuid.UUID
	Status         *string
	PaymentTerms   *string
	Search         *string
	Module         *string
	DueDateStart   *time.Time
	DueDateEnd     *time.Time
	MinAmount      *float64
	MaxAmount      *float64
	CreatedAtStart *time.Time
	CreatedAtEnd   *time.Time
}

