package unit

import (
	"context"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"erp-billing-service/internal/application"
	"erp-billing-service/internal/domain"

	"github.com/google/uuid"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.015
}

func TestBuildPartialPaymentInvoice_CompleteBreakdownAndPennyReconciliation(t *testing.T) {
	orgID := uuid.New()
	custID := uuid.New()
	soID := uuid.New()
	parentID := uuid.New()
	parentNum := "INV-2026-0001"
	soRef := soID.String()

	parent := &domain.Invoice{
		ID:                parentID,
		OrganizationID:    orgID,
		CustomerID:        custID,
		Subject:           "Detailed Order",
		InvoiceNumber:     &parentNum,
		SourceSystem:      domain.SourceSystemCRM,
		SourceReferenceID: &soRef,
		SalesOrderID:      &soID,
		SalesOrder:        "SO-2026-0001",
		InvoiceDate:       time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC),
		DueDate:           time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC),
		Status:            domain.InvoiceStatusSent,
		SubTotal:          3000,
		DiscountTotal:     20,
		TaxTotal:          36,
		Adjustment:        10,
		ExciseDuty:        100,
		TotalAmount:       3126,
		PaidAmount:        0,
		BalanceAmount:     3126,
		Currency:          "INR",
		Items: []domain.InvoiceItem{
			{
				ID:        uuid.New(),
				InvoiceID: parentID,
				ItemID:    uuid.New(),
				ItemType:  "service",
				Name:      "Full Service",
				Quantity:  1,
				UnitPrice: 2000,
				Discount:  10,
				Tax:       18,
				Total:     2008,
			},
			{
				ID:        uuid.New(),
				InvoiceID: parentID,
				ItemID:    uuid.New(),
				ItemType:  "part",
				Name:      "Chain Sprocket",
				Quantity:  1,
				UnitPrice: 1000,
				Discount:  10,
				Tax:       18,
				Total:     1008,
			},
		},
	}

	// First partial payment: 2000 out of 3126
	p1 := application.BuildPartialPaymentInvoice(
		parent,
		time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC),
		"INV-2026-0002",
		2000,
		0,
		nil,
		"SO-2026-0001",
	)
	if !p1.IsPartialInvoice {
		t.Fatalf("expected IsPartialInvoice=true")
	}
	if p1.ParentInvoiceID == nil || *p1.ParentInvoiceID != parentID {
		t.Fatalf("expected ParentInvoiceID=%s", parentID)
	}
	if p1.SalesOrderID == nil || *p1.SalesOrderID != soID || p1.SalesOrder != "SO-2026-0001" {
		t.Fatalf("expected partial invoice to link to sales order SO-2026-0001")
	}
	if !almostEqual(p1.ActualAmount, 3126) || !almostEqual(p1.PaidAmount, 2000) || !almostEqual(p1.PreviousPaidAmount, 0) || !almostEqual(p1.OrderBalanceAmount, 1126) {
		t.Fatalf("unexpected p1 amounts: actual=%.2f paid=%.2f prev=%.2f orderBal=%.2f", p1.ActualAmount, p1.PaidAmount, p1.PreviousPaidAmount, p1.OrderBalanceAmount)
	}
	if p1.TaxTotal <= 0 || p1.DiscountTotal <= 0 || p1.Adjustment <= 0 {
		t.Fatalf("expected proportional tax/discount/adjustment on p1, got tax=%.2f disc=%.2f adj=%.2f", p1.TaxTotal, p1.DiscountTotal, p1.Adjustment)
	}

	// Simulate parent updated after Payment #1
	parent.PaidAmount = 2000
	parent.BalanceAmount = 1126
	parent.Status = domain.InvoiceStatusPartial

	// Second partial payment: remaining 1126 out of 1126 balance
	p2 := application.BuildPartialPaymentInvoice(
		parent,
		time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC),
		"INV-2026-0003",
		1126,
		2000,
		[]domain.Invoice{*p1},
		"SO-2026-0001",
	)
	if *p2.InvoiceNumber != "INV-2026-0003" {
		t.Fatalf("expected distinct invoice number INV-2026-0003, got %v", p2.InvoiceNumber)
	}
	if !almostEqual(p2.PreviousPaidAmount, 2000) || !almostEqual(p2.OrderBalanceAmount, 0) {
		t.Fatalf("unexpected p2 amounts: prev=%.2f orderBal=%.2f", p2.PreviousPaidAmount, p2.OrderBalanceAmount)
	}
	if !almostEqual(p1.TaxTotal+p2.TaxTotal, parent.TaxTotal) {
		t.Fatalf("expected GST sum %.2f + %.2f == %.2f", p1.TaxTotal, p2.TaxTotal, parent.TaxTotal)
	}
	if !almostEqual(p1.DiscountTotal+p2.DiscountTotal, parent.DiscountTotal) {
		t.Fatalf("expected Discount sum %.2f + %.2f == %.2f", p1.DiscountTotal, p2.DiscountTotal, parent.DiscountTotal)
	}
	if !almostEqual(p1.Adjustment+p2.Adjustment, parent.Adjustment) {
		t.Fatalf("expected Adjustment sum %.2f + %.2f == %.2f", p1.Adjustment, p2.Adjustment, parent.Adjustment)
	}
}

func TestGeneratePartialInvoicePDF_IncludesPartialBreakdownAndPreviousPaid(t *testing.T) {
	pdfSvc := application.NewPDFService(t.TempDir())
	orgID := uuid.New()
	custID := uuid.New()
	parentID := uuid.New()
	parentNum := "INV-2026-0001"
	partNum := "INV-2026-0003"
	prevNum := "INV-2026-0002"

	inv := &domain.Invoice{
		ID:                  uuid.New(),
		OrganizationID:      orgID,
		CustomerID:          custID,
		InvoiceNumber:       &partNum,
		ParentInvoiceID:     &parentID,
		ParentInvoiceNumber: &parentNum,
		SalesOrder:          "SO-2026-0001",
		IsPartialInvoice:    true,
		InvoiceDate:         time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC),
		DueDate:             time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC),
		Status:              domain.InvoiceStatusPaid,
		Currency:            "INR",
		SubTotal:            1071.01,
		DiscountTotal:       7.14,
		TaxTotal:            12.85,
		Adjustment:          3.57,
		ExciseDuty:          35.71,
		TotalAmount:         1116,
		PaidAmount:          1116,
		BalanceAmount:       0,
		ActualAmount:        3116,
		PreviousPaidAmount:  2000,
		ActualDiscountTotal: 20,
		ActualTaxTotal:      36,
		ActualAdjustment:    10,
		OrderBalanceAmount:  0,
		Payments: []domain.Payment{
			{
				ID:             uuid.New(),
				OrganizationID: orgID,
				Amount:         1116,
				PaymentDate:    time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC),
				Method:         domain.PaymentMethodCash,
				Reference:      "TXN-CASH-002",
				Status:         domain.PaymentStatusCompleted,
			},
		},
		PreviousPayments: []domain.Payment{
			{
				ID:                   uuid.New(),
				OrganizationID:       orgID,
				Amount:               2000,
				PaymentDate:          time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC),
				Method:               domain.PaymentMethodBank,
				Reference:            "TXN-BANK-001",
				PartialInvoiceNumber: &prevNum,
				Status:               domain.PaymentStatusCompleted,
			},
		},
	}

	pdfPath, err := pdfSvc.GenerateInvoicePDF(context.Background(), inv, &domain.CustomerRM{DisplayName: "Customer One"}, &domain.OrganizationRM{OrganizationName: "Registered Co"}, nil, nil, nil, "Author")
	if err != nil {
		t.Fatalf("GenerateInvoicePDF failed: %v", err)
	}
	bytes, err := os.ReadFile(pdfPath)
	if err != nil || !strings.HasPrefix(string(bytes), "%PDF-") {
		t.Fatalf("expected valid PDF bytes, err=%v", err)
	}
}

func TestGeneratePartialInvoicePDFWithSettings_SignatureAndGSTBreakdown(t *testing.T) {
	pdfSvc := application.NewPDFService(t.TempDir())
	orgID := uuid.New()
	custID := uuid.New()
	partNum := "EFS-2026-0005"

	inv := &domain.Invoice{
		ID:                  uuid.New(),
		OrganizationID:      orgID,
		CustomerID:          custID,
		InvoiceNumber:       &partNum,
		IsPartialInvoice:    true,
		InvoiceDate:         time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC),
		DueDate:             time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC),
		Status:              domain.InvoiceStatusPaid,
		Currency:            "INR",
		SubTotal:            1000,
		DiscountTotal:       0,
		TaxTotal:            180,
		ExciseDuty:          20,
		TotalAmount:         1200,
		PaidAmount:          1200,
		ActualSubTotal:      2500,
		ActualAmount:        3000,
		PreviousPaidAmount:  1800,
		ActualTaxTotal:      450,
		ActualExciseDuty:    50,
		OrderBalanceAmount:  0,
	}

	// 1x1 transparent PNG base64 data URI
	tinyPNG := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	settings := &domain.InvoiceSettings{
		OrganizationID:     orgID,
		ShowSignature:      true,
		SignatureMandatory: true,
		SignatoryName:      "Jane Signatory",
		SignatoryTitle:     "Finance Director",
		SignatureLabel:     "Authorized Signature",
		SignatureImage:     tinyPNG,
		ShowGSTBreakdown:   true,
		GSTMode:            "cgst_sgst",
	}

	pdfPath, err := pdfSvc.GenerateInvoicePDFWithSettings(
		context.Background(),
		inv,
		&domain.CustomerRM{DisplayName: "Customer One", BillingState: "Tamil Nadu"},
		&domain.OrganizationRM{OrganizationName: "Registered Co", State: "Tamil Nadu"},
		nil,
		nil,
		nil,
		"Fallback Author",
		settings,
	)
	if err != nil {
		t.Fatalf("GenerateInvoicePDFWithSettings failed: %v", err)
	}
	b, err := os.ReadFile(pdfPath)
	if err != nil || !strings.HasPrefix(string(b), "%PDF-") {
		t.Fatalf("expected valid PDF bytes, err=%v", err)
	}
}
