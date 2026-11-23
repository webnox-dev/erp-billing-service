package unit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	httpHandler "erp-billing-service/internal/adapters/inbound/http"
	"erp-billing-service/internal/application"
	"erp-billing-service/internal/application/dto"
	"erp-billing-service/internal/domain"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type stubInvoiceRepo struct {
	domain.InvoiceRepository
	inv               *domain.Invoice
	getNextNumCalled  bool
	updateCalled      bool
}

func (r *stubInvoiceRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Invoice, error) {
	if r.inv != nil && r.inv.ID == id {
		return r.inv, nil
	}
	return nil, nil
}

func (r *stubInvoiceRepo) GetNextInvoiceNumber(ctx context.Context, orgID uuid.UUID) (string, error) {
	r.getNextNumCalled = true
	return "INV-2026-0001", nil
}

func (r *stubInvoiceRepo) Update(ctx context.Context, invoice *domain.Invoice) error {
	r.updateCalled = true
	return nil
}

type stubReadModelRepo struct {
	domain.ReadModelRepository
	customer       *domain.CustomerRM
	customerErr    error
	contact        *domain.ContactRM
	primaryContact *domain.ContactRM
}

func (r *stubReadModelRepo) GetCustomer(ctx context.Context, id uuid.UUID) (*domain.CustomerRM, error) {
	if r.customerErr != nil {
		return nil, r.customerErr
	}
	return r.customer, nil
}

func (r *stubReadModelRepo) GetContact(ctx context.Context, id uuid.UUID) (*domain.ContactRM, error) {
	return r.contact, nil
}

func (r *stubReadModelRepo) GetPrimaryContact(ctx context.Context, customerID uuid.UUID) (*domain.ContactRM, error) {
	return r.primaryContact, nil
}

func newDraftTestInvoice(orgID, custID uuid.UUID) *domain.Invoice {
	return &domain.Invoice{
		ID:             uuid.New(),
		OrganizationID: orgID,
		CustomerID:     custID,
		Subject:        "Test Invoice",
		Status:         domain.InvoiceStatusDraft,
		InvoiceDate:    time.Now().UTC(),
		DueDate:        time.Now().UTC().Add(24 * time.Hour),
		Currency:       "USD",
	}
}

func TestSendInvoice_RejectsIncompleteCustomerAndMissingEmail(t *testing.T) {
	orgID := uuid.New()
	custID := uuid.New()

	tests := []struct {
		name        string
		rm          *stubReadModelRepo
		custID      uuid.UUID
		wantErrPart string
	}{
		{
			name: "missing customer id",
			rm:   &stubReadModelRepo{},
			custID: uuid.Nil,
			wantErrPart: "customer is required",
		},
		{
			name: "missing customer record",
			rm: &stubReadModelRepo{
				customer:    nil,
				customerErr: errors.New("not found"),
			},
			custID:      custID,
			wantErrPart: "customer details could not be found",
		},
		{
			name: "placeholder unknown customer name",
			rm: &stubReadModelRepo{
				customer: &domain.CustomerRM{
					ID:             custID,
					OrganizationID: orgID,
					DisplayName:    "Unknown Customer",
					Email:          "client@example.com",
				},
			},
			custID:      custID,
			wantErrPart: "valid customer name is required",
		},
		{
			name: "empty customer name with no company or person name",
			rm: &stubReadModelRepo{
				customer: &domain.CustomerRM{
					ID:             custID,
					OrganizationID: orgID,
					Email:          "client@example.com",
				},
			},
			custID:      custID,
			wantErrPart: "valid customer name is required",
		},
		{
			name: "valid customer without customer or contact email",
			rm: &stubReadModelRepo{
				customer: &domain.CustomerRM{
					ID:             custID,
					OrganizationID: orgID,
					DisplayName:    "Acme Services",
				},
			},
			custID:      custID,
			wantErrPart: "email address is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inv := newDraftTestInvoice(orgID, tc.custID)
			invRepo := &stubInvoiceRepo{inv: inv}
			svc := application.NewInvoiceService(invRepo, tc.rm, nil, nil, nil, nil, nil, nil, nil, nil)

			_, err := svc.SendInvoice(context.Background(), inv.ID, dto.SendInvoiceRequest{})
			if err == nil {
				t.Fatalf("expected SendInvoice to fail with %q, got nil", tc.wantErrPart)
			}
			if !strings.HasPrefix(err.Error(), "cannot send") || !strings.Contains(err.Error(), tc.wantErrPart) {
				t.Fatalf("unexpected error: %v (wanted substring %q)", err, tc.wantErrPart)
			}
			if invRepo.getNextNumCalled || invRepo.updateCalled {
				t.Fatalf("expected invoice number/update not to run on invalid send")
			}
			if inv.Status != domain.InvoiceStatusDraft {
				t.Fatalf("expected invoice status to remain draft, got %s", inv.Status)
			}
		})
	}
}

func TestSendInvoiceHandler_ReturnsBadRequestJSONOnValidationFailure(t *testing.T) {
	orgID := uuid.New()
	custID := uuid.New()
	inv := newDraftTestInvoice(orgID, custID)
	invRepo := &stubInvoiceRepo{inv: inv}
	rmRepo := &stubReadModelRepo{
		customer: &domain.CustomerRM{
			ID:             custID,
			OrganizationID: orgID,
			DisplayName:    "Informal Client",
		},
	}

	svc := application.NewInvoiceService(invRepo, rmRepo, nil, nil, nil, nil, nil, nil, nil, nil)
	h := httpHandler.NewInvoiceHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/invoices/"+inv.ID.String()+"/send", http.NoBody)
	req = mux.SetURLVars(req, map[string]string{"id": inv.ID.String()})
	rr := httptest.NewRecorder()

	h.SendInvoice(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d (body: %s)", rr.Code, rr.Body.String())
	}
	var payload map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON error payload, decode error: %v", err)
	}
	if !strings.Contains(payload["error"], "email address is required") {
		t.Fatalf("unexpected JSON error payload: %+v", payload)
	}
}
