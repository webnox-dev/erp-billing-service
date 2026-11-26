package application

import (
	"context"
	"fmt"
	"strings"

	"erp-billing-service/internal/domain"

	"github.com/google/uuid"
)

// InvoiceSettingsService manages per-organization invoice configuration and signature rules.
type InvoiceSettingsService struct {
	repo domain.InvoiceSettingsRepository
}

func NewInvoiceSettingsService(repo domain.InvoiceSettingsRepository) *InvoiceSettingsService {
	return &InvoiceSettingsService{repo: repo}
}

func (s *InvoiceSettingsService) GetSettings(ctx context.Context, orgID uuid.UUID) (*domain.InvoiceSettings, error) {
	if orgID == uuid.Nil {
		return domain.DefaultInvoiceSettings(orgID), nil
	}
	settings, err := s.repo.GetByOrganizationID(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get invoice settings: %w", err)
	}
	if settings == nil {
		return domain.DefaultInvoiceSettings(orgID), nil
	}
	if strings.TrimSpace(settings.SignatureLabel) == "" {
		settings.SignatureLabel = "Authorized Signature"
	}
	if strings.TrimSpace(settings.GSTMode) == "" {
		settings.GSTMode = "auto"
	}
	if strings.TrimSpace(settings.InvoicePrefix) == "" {
		settings.InvoicePrefix = "INV"
	}
	if strings.TrimSpace(settings.DefaultPaymentTerms) == "" {
		settings.DefaultPaymentTerms = "Due on Receipt"
	}
	return settings, nil
}

func (s *InvoiceSettingsService) UpdateSettings(ctx context.Context, orgID uuid.UUID, req *domain.InvoiceSettings) (*domain.InvoiceSettings, error) {
	if orgID == uuid.Nil {
		return nil, fmt.Errorf("organization_id is required")
	}
	if req == nil {
		return nil, fmt.Errorf("invalid settings payload")
	}

	req.OrganizationID = orgID
	req.SignatoryName = strings.TrimSpace(req.SignatoryName)
	req.SignatoryTitle = strings.TrimSpace(req.SignatoryTitle)
	req.SignatureLabel = strings.TrimSpace(req.SignatureLabel)
	if req.SignatureLabel == "" {
		req.SignatureLabel = "Authorized Signature"
	}

	req.GSTMode = strings.ToLower(strings.TrimSpace(req.GSTMode))
	switch req.GSTMode {
	case "auto", "cgst_sgst", "igst":
	default:
		req.GSTMode = "auto"
	}

	req.InvoicePrefix = strings.ToUpper(strings.TrimSpace(req.InvoicePrefix))
	req.InvoicePrefix = strings.TrimSuffix(req.InvoicePrefix, "-")
	if req.InvoicePrefix == "" {
		req.InvoicePrefix = "INV"
	}
	if len(req.InvoicePrefix) > 15 {
		req.InvoicePrefix = req.InvoicePrefix[:15]
	}

	if strings.TrimSpace(req.DefaultPaymentTerms) == "" {
		req.DefaultPaymentTerms = "Due on Receipt"
	}

	// Mandatory signature implies showing signature on invoices and requires a configured signature image or signatory name.
	if req.SignatureMandatory {
		req.ShowSignature = true
		if strings.TrimSpace(req.SignatureImage) == "" && req.SignatoryName == "" {
			return nil, fmt.Errorf("validation error: please provide a signature image or authorized signatory name when signature is mandatory")
		}
	}

	if err := s.repo.Upsert(ctx, req); err != nil {
		return nil, fmt.Errorf("failed to save invoice settings: %w", err)
	}
	return s.GetSettings(ctx, orgID)
}
