package postgres

import (
	"strings"
	"testing"

	"erp-billing-service/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/utils/tests"
)

func TestApplyPaymentFilters_SQLColumnsAndParentheses(t *testing.T) {
	db, err := gorm.Open(tests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("open dryrun gorm db: %v", err)
	}

	orgID := uuid.New()
	customerName := "Gokul"
	method := "manual"
	status := "completed"
	search := "INV-2026"
	invNum := "INV-2026-0001"
	minAmt := -500.0
	maxAmt := 2000.0

	filter := domain.PaymentFilter{
		OrganizationID: orgID,
		CustomerName:   &customerName,
		Method:         &method,
		Status:         &status,
		Search:         &search,
		InvoiceNumber:  &invNum,
		MinAmount:      &minAmt,
		MaxAmount:      &maxAmt,
	}

	sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		var rows []domain.Payment
		return applyPaymentFilters(
			tx.Joins("JOIN invoices ON invoices.id = payments.invoice_id"),
			filter,
		).Find(&rows)
	})

	for _, forbidden := range []string{"payments.method", "payments.reference", "payments.customer_name"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("expected SQL not to contain %q, got: %s", forbidden, sql)
		}
	}

	for _, required := range []string{
		"payments.payment_method",
		"payments.transaction_ref",
		"payments.partial_invoice_number",
		"customers_readonly",
		"(invoices.invoice_number ILIKE",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("expected SQL to contain %q, got: %s", required, sql)
		}
	}
}
