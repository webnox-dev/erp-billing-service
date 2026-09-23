package postgres

import (
	"context"

	"erp-billing-service/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PaymentRepository struct {
	db *gorm.DB
}

func NewPaymentRepository(db *gorm.DB) *PaymentRepository {
	return &PaymentRepository{db: db}
}

func (r *PaymentRepository) Create(ctx context.Context, payment *domain.Payment) error {
	return r.db.WithContext(ctx).Create(payment).Error
}

func (r *PaymentRepository) GetByInvoiceID(ctx context.Context, invoiceID uuid.UUID) ([]domain.Payment, error) {
	var payments []domain.Payment
	err := r.db.WithContext(ctx).Where("invoice_id = ?", invoiceID).Find(&payments).Error
	return payments, err
}

func (r *PaymentRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	var payment domain.Payment
	err := r.db.WithContext(ctx).First(&payment, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &payment, nil
}

func (r *PaymentRepository) Update(ctx context.Context, payment *domain.Payment) error {
	return r.db.WithContext(ctx).Save(payment).Error
}

func (r *PaymentRepository) ListByInvoice(ctx context.Context, invoiceID uuid.UUID) ([]domain.Payment, error) {
	var payments []domain.Payment
	err := r.db.WithContext(ctx).Where("invoice_id = ?", invoiceID).Order("payment_date desc").Find(&payments).Error
	return payments, err
}

func (r *PaymentRepository) List(ctx context.Context) ([]domain.Payment, error) {
	var payments []domain.Payment
	err := r.db.WithContext(ctx).Order("payment_date desc").Find(&payments).Error
	return payments, err
}

// ListByModule returns payments filtered by invoice source_system
func (r *PaymentRepository) ListByModule(ctx context.Context, orgID uuid.UUID, sourceSystem domain.SourceSystem) ([]domain.Payment, error) {
	var payments []domain.Payment
	
	// Join with invoices table to filter by source_system
	query := r.db.WithContext(ctx).
		Joins("JOIN invoices ON invoices.id = payments.invoice_id")
	if orgID != uuid.Nil {
		query = query.Where("payments.organization_id = ?", orgID)
	}
	err := query.Where("invoices.source_system = ?", sourceSystem).
		Order("payments.payment_date desc").
		Find(&payments).Error
	
	return payments, err
}

// ListWithFilters returns payments filtered by domain.PaymentFilter
func (r *PaymentRepository) ListWithFilters(ctx context.Context, filter domain.PaymentFilter) ([]domain.Payment, error) {
	var payments []domain.Payment
	query := r.db.WithContext(ctx).
		Joins("JOIN invoices ON invoices.id = payments.invoice_id")

	if filter.OrganizationID != uuid.Nil {
		query = query.Where("payments.organization_id = ?", filter.OrganizationID)
	}
	if filter.SourceSystem != nil && *filter.SourceSystem != "" {
		query = query.Where("invoices.source_system = ?", *filter.SourceSystem)
	}
	if filter.CustomerID != nil {
		query = query.Where("invoices.customer_id = ?", *filter.CustomerID)
	}
	if filter.CustomerName != nil && *filter.CustomerName != "" {
		custPattern := "%" + *filter.CustomerName + "%"
		query = query.Where("payments.customer_name ILIKE ? OR invoices.customer_id IN (SELECT id FROM customers_readonly WHERE display_name ILIKE ? OR company_name ILIKE ?)", custPattern, custPattern, custPattern)
	}
	if filter.Status != nil && *filter.Status != "" {
		query = query.Where("LOWER(payments.status) = LOWER(?)", *filter.Status)
	}
	if filter.Method != nil && *filter.Method != "" {
		query = query.Where("LOWER(payments.method) = LOWER(?)", *filter.Method)
	}
	if filter.Search != nil && *filter.Search != "" {
		pattern := "%" + *filter.Search + "%"
		query = query.Where("invoices.invoice_number ILIKE ? OR payments.reference ILIKE ? OR payments.customer_name ILIKE ? OR invoices.customer_id IN (SELECT id FROM customers_readonly WHERE display_name ILIKE ? OR company_name ILIKE ?)", pattern, pattern, pattern, pattern, pattern)
	}
	if filter.InvoiceNumber != nil && *filter.InvoiceNumber != "" {
		invPattern := "%" + *filter.InvoiceNumber + "%"
		query = query.Where("invoices.invoice_number ILIKE ?", invPattern)
	}
	if filter.PaymentDateStart != nil {
		query = query.Where("payments.payment_date >= ?", *filter.PaymentDateStart)
	}
	if filter.PaymentDateEnd != nil {
		query = query.Where("payments.payment_date <= ?", *filter.PaymentDateEnd)
	}
	if filter.MinAmount != nil {
		query = query.Where("payments.amount >= ?", *filter.MinAmount)
	}
	if filter.MaxAmount != nil {
		query = query.Where("payments.amount <= ?", *filter.MaxAmount)
	}
	if filter.CreatedAtStart != nil {
		query = query.Where("payments.created_at >= ?", *filter.CreatedAtStart)
	}
	if filter.CreatedAtEnd != nil {
		query = query.Where("payments.created_at <= ?", *filter.CreatedAtEnd)
	}

	err := query.Order("payments.payment_date desc, payments.created_at desc").Find(&payments).Error
	return payments, err
}


func (r *PaymentRepository) ListByCustomerIDs(ctx context.Context, orgID uuid.UUID, customerIDs []uuid.UUID) ([]domain.Payment, error) {
	var payments []domain.Payment
	query := r.db.WithContext(ctx).
		Joins("JOIN invoices ON invoices.id = payments.invoice_id")
	if orgID != uuid.Nil {
		query = query.Where("payments.organization_id = ?", orgID)
	}
	err := query.Where("invoices.customer_id IN ?", customerIDs).
		Order("payments.payment_date desc").
		Find(&payments).Error
	return payments, err
}


