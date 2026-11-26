package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// InvoiceSettings stores per-organization invoice branding, GST breakdown, defaults, and signature policy.
type InvoiceSettings struct {
	ID                  uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID      uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"organization_id"`
	ShowSignature       bool      `gorm:"default:true" json:"show_signature"`
	SignatureMandatory  bool      `gorm:"default:false" json:"signature_mandatory"`
	SignatoryName       string    `gorm:"type:varchar(255)" json:"signatory_name"`
	SignatoryTitle      string    `gorm:"type:varchar(255)" json:"signatory_title"`
	SignatureLabel      string    `gorm:"type:varchar(100);default:'Authorized Signature'" json:"signature_label"`
	SignatureImage      string    `gorm:"type:text" json:"signature_image"`
	ShowGSTBreakdown    bool      `gorm:"default:true" json:"show_gst_breakdown"`
	GSTMode             string    `gorm:"type:varchar(30);default:'auto'" json:"gst_mode"`
	InvoicePrefix       string    `gorm:"type:varchar(20);default:'INV'" json:"invoice_prefix"`
	DefaultPaymentTerms string    `gorm:"type:varchar(100);default:'Due on Receipt'" json:"default_payment_terms"`
	DefaultNotes        string    `gorm:"type:text" json:"default_notes"`
	DefaultTerms        string    `gorm:"type:text" json:"default_terms"`
	CompanyName         string    `gorm:"type:varchar(255)" json:"company_name"`
	CompanyAddress      string    `gorm:"type:text" json:"company_address"`
	CompanyCity         string    `gorm:"type:varchar(100)" json:"company_city"`
	CompanyState        string    `gorm:"type:varchar(100)" json:"company_state"`
	CompanyZipCode      string    `gorm:"type:varchar(20)" json:"company_zip_code"`
	CompanyCountry      string    `gorm:"type:varchar(100)" json:"company_country"`
	CompanyPhone        string    `gorm:"type:varchar(50)" json:"company_phone"`
	CompanyEmail        string    `gorm:"type:varchar(255)" json:"company_email"`
	CompanyWebsite      string    `gorm:"type:text" json:"company_website"`
	CompanyGSTIN        string    `gorm:"type:varchar(50)" json:"company_gstin"`
	CompanyLogoURL      string    `gorm:"type:text" json:"company_logo_url"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (InvoiceSettings) TableName() string {
	return "invoice_settings"
}

// DefaultInvoiceSettings returns default settings for an organization when none are saved yet.
func DefaultInvoiceSettings(orgID uuid.UUID) *InvoiceSettings {
	return &InvoiceSettings{
		OrganizationID:      orgID,
		ShowSignature:       true,
		SignatureMandatory:  false,
		SignatureLabel:      "Authorized Signature",
		ShowGSTBreakdown:    true,
		GSTMode:             "auto",
		InvoicePrefix:       "INV",
		DefaultPaymentTerms: "Due on Receipt",
	}
}

type InvoiceSettingsRepository interface {
	GetByOrganizationID(ctx context.Context, orgID uuid.UUID) (*InvoiceSettings, error)
	Upsert(ctx context.Context, settings *InvoiceSettings) error
}
