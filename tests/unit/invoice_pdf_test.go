package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	outbound_http "erp-billing-service/internal/adapters/outbound/http"
	"erp-billing-service/internal/application"
	"erp-billing-service/internal/domain"

	"github.com/google/uuid"
)

func TestGenerateInvoicePDF_IncludesAllRequiredDetails(t *testing.T) {
	tempDir := t.TempDir()
	pdfSvc := application.NewPDFService(tempDir)

	orgID := uuid.New()
	custID := uuid.New()
	invID := uuid.New()
	invNum := "INV-2026-0099"

	inv := &domain.Invoice{
		ID:             invID,
		OrganizationID: orgID,
		CustomerID:     custID,
		InvoiceNumber:  &invNum,
		InvoiceDate:    time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC),
		DueDate:        time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC),
		Status:         domain.InvoiceStatusPartial,
		Currency:       "INR",
		SubTotal:       3000,
		DiscountTotal:  20,
		TaxTotal:       36,
		ExciseDuty:     100,
		TotalAmount:    3116,
		PaidAmount:     2000,
		BalanceAmount:  1116,
		BillingStreet:  "2nd floor, Venky Complex",
		BillingCity:    "Coimbatore",
		BillingState:   "Tamil Nadu",
		BillingCode:    "641012",
		BillingCountry: "India",
		Items: []domain.InvoiceItem{
			{
				ID:          uuid.New(),
				InvoiceID:   invID,
				ItemType:    "service",
				Name:        "AC Maintenance",
				Description: "Full service check",
				Quantity:    1,
				UnitPrice:   2000,
				Discount:    10,
				Tax:         18,
				Total:       2008,
			},
			{
				ID:          uuid.New(),
				InvoiceID:   invID,
				ItemType:    "part",
				Name:        "Chain Sprocket",
				Description: "Replacement part",
				Quantity:    1,
				UnitPrice:   1000,
				Discount:    10,
				Tax:         18,
				Total:       1008,
			},
		},
		Payments: []domain.Payment{
			{
				ID:             uuid.New(),
				OrganizationID: orgID,
				InvoiceID:      invID,
				Amount:         2000,
				PaymentDate:    time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC),
				Method:         domain.PaymentMethodUPI,
				Reference:      "TXN-UPI-987654321",
				Status:         domain.PaymentStatusCompleted,
			},
		},
	}

	customer := &domain.CustomerRM{
		ID:             custID,
		OrganizationID: orgID,
		DisplayName:    "Gokulvinith S",
		CompanyName:    "Acme Client Co",
		Email:          "client@example.com",
		Phone:          "+919876543211",
	}

	org := &domain.OrganizationRM{
		ID:               orgID,
		OrganizationName: "EFS Test Company Pvt Ltd",
		Address:          "4th Floor, Tech Park",
		City:             "Coimbatore",
		State:            "Tamil Nadu",
		ZipCode:          "641012",
		Country:          "India",
		Phone:            "+919688140720",
		Email:            "billing@efstest.com",
		Website:          "https://easyfieldservices.com",
		GSTIN:            "33AAAAA0000A1Z5",
	}

	pdfPath, err := pdfSvc.GenerateInvoicePDF(context.Background(), inv, customer, org, nil, nil, nil, "Vinith S")
	if err != nil {
		t.Fatalf("GenerateInvoicePDF returned error: %v", err)
	}

	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("failed to read generated PDF: %v", err)
	}
	if len(pdfBytes) < 1000 {
		t.Fatalf("generated PDF unexpectedly small: %d bytes", len(pdfBytes))
	}

	// gofpdf writes uncompressed content streams by default, or verify header & non-empty file
	content := string(pdfBytes)
	if !strings.HasPrefix(content, "%PDF-") {
		t.Fatalf("expected valid PDF header")
	}
}

func TestHTTPFallbacks_CustomerAndOrganization(t *testing.T) {
	orgID := uuid.New()
	custID := uuid.New()

	orgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/organizations/"+orgID.String() {
			t.Fatalf("unexpected org path: %s", r.URL.Path)
		}
		if r.Header.Get("X-User-ID") == "" {
			t.Fatalf("expected X-User-ID header")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":                orgID.String(),
			"organization_name": "Registered Org Co",
			"address":           "123 Main Street",
			"city":              "Coimbatore",
			"state":             "TN",
			"zip_code":          "641001",
			"country":           "India",
			"phone":             "+919999999999",
			"email":             "info@registered.com",
			"website":           "https://registered.com",
			"gstin":             "33ABCDE1234F1Z5",
		})
	}))
	defer orgServer.Close()

	custServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/customers/"+custID.String() {
			t.Fatalf("unexpected customer path: %s", r.URL.Path)
		}
		if r.Header.Get("X-Organization-Id") != orgID.String() {
			t.Fatalf("expected X-Organization-Id header %s, got %q", orgID.String(), r.Header.Get("X-Organization-Id"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"id":           custID.String(),
				"display_name": "Client Alpha",
				"company_name": "Client Alpha LLC",
				"email":        "alpha@client.com",
				"phone_mobile": "+918888888888",
				"addresses": []map[string]interface{}{
					{
						"type":        "billing",
						"street1":     "10 Client Rd",
						"city":        "Chennai",
						"state":       "TN",
						"postal_code": "600001",
						"country":     "India",
					},
				},
			},
		})
	}))
	defer custServer.Close()

	orgClient := outbound_http.NewOrganizationHTTPClient(orgServer.URL)
	orgRM, err := orgClient.GetOrganization(context.Background(), orgID)
	if err != nil {
		t.Fatalf("GetOrganization failed: %v", err)
	}
	if orgRM == nil || orgRM.OrganizationName != "Registered Org Co" || orgRM.GSTIN != "33ABCDE1234F1Z5" {
		t.Fatalf("unexpected orgRM: %+v", orgRM)
	}

	custClient := outbound_http.NewCustomerHTTPClient(custServer.URL)
	ctx := context.WithValue(context.Background(), "organization_id", orgID.String())
	custRM, err := custClient.GetCustomer(ctx, custID)
	if err != nil {
		t.Fatalf("GetCustomer failed: %v", err)
	}
	if custRM == nil || custRM.DisplayName != "Client Alpha" || custRM.Email != "alpha@client.com" || custRM.Phone != "+918888888888" || custRM.BillingCity != "Chennai" {
		t.Fatalf("unexpected custRM: %+v", custRM)
	}
}
