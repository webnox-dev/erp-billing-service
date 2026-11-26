package postgres

import (
	"context"
	"errors"
	"time"

	"erp-billing-service/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type InvoiceSettingsRepository struct {
	db *gorm.DB
}

func NewInvoiceSettingsRepository(db *gorm.DB) *InvoiceSettingsRepository {
	return &InvoiceSettingsRepository{db: db}
}

func (r *InvoiceSettingsRepository) GetByOrganizationID(ctx context.Context, orgID uuid.UUID) (*domain.InvoiceSettings, error) {
	if orgID == uuid.Nil {
		return nil, nil
	}
	var settings domain.InvoiceSettings
	err := r.db.WithContext(ctx).First(&settings, "organization_id = ?", orgID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &settings, nil
}

func (r *InvoiceSettingsRepository) Upsert(ctx context.Context, settings *domain.InvoiceSettings) error {
	if settings == nil || settings.OrganizationID == uuid.Nil {
		return errors.New("organization_id is required")
	}

	var existing domain.InvoiceSettings
	err := r.db.WithContext(ctx).First(&existing, "organization_id = ?", settings.OrganizationID).Error
	now := time.Now().UTC()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if settings.ID == uuid.Nil {
			settings.ID = uuid.New()
		}
		settings.CreatedAt = now
		settings.UpdatedAt = now
		return r.db.WithContext(ctx).Create(settings).Error
	}
	if err != nil {
		return err
	}

	settings.ID = existing.ID
	settings.CreatedAt = existing.CreatedAt
	settings.UpdatedAt = now
	return r.db.WithContext(ctx).Save(settings).Error
}
