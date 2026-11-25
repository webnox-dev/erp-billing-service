package http

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"erp-billing-service/internal/application"
	"erp-billing-service/internal/application/dto"
	"erp-billing-service/internal/domain"

	shared_events "github.com/efs/shared-events"
	"github.com/google/uuid"
)

type stubSalesOrderRepo struct {
	created []*domain.SalesOrder
}

func (s *stubSalesOrderRepo) Create(order *domain.SalesOrder) error {
	s.created = append(s.created, order)
	return nil
}
func (s *stubSalesOrderRepo) Update(order *domain.SalesOrder) error               { return nil }
func (s *stubSalesOrderRepo) FindByID(id uuid.UUID) (*domain.SalesOrder, error)   { return nil, nil }
func (s *stubSalesOrderRepo) FindByOrderNumber(num string) (*domain.SalesOrder, error) {
	return nil, nil
}
func (s *stubSalesOrderRepo) List(orgID uuid.UUID, filters *dto.SalesOrderFilters) ([]*domain.SalesOrder, int64, error) {
	return nil, 0, nil
}
func (s *stubSalesOrderRepo) Delete(id uuid.UUID) error                     { return nil }
func (s *stubSalesOrderRepo) GenerateOrderNumber(orgID uuid.UUID) (string, error) { return "SO-1", nil }
func (s *stubSalesOrderRepo) FindByInvoiceID(id uuid.UUID) (*domain.SalesOrder, error) {
	return nil, nil
}
func (s *stubSalesOrderRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.SalesOrderStatus) error {
	return nil
}

type stubEventPublisher struct{}

func (s *stubEventPublisher) Publish(ctx context.Context, metadata shared_events.EventMetadata, payload interface{}) error {
	return nil
}

type stubReadModelRepo struct{}

func (s *stubReadModelRepo) GetCustomer(ctx context.Context, id uuid.UUID) (*domain.CustomerRM, error) {
	return &domain.CustomerRM{ID: id, DisplayName: "Test Customer"}, nil
}
func (s *stubReadModelRepo) UpsertCustomer(ctx context.Context, rm *domain.CustomerRM) error {
	return nil
}
func (s *stubReadModelRepo) SearchCustomers(ctx context.Context, orgID uuid.UUID, query string) ([]domain.CustomerRM, error) {
	return nil, nil
}
func (s *stubReadModelRepo) GetItem(ctx context.Context, id uuid.UUID) (*domain.ItemRM, error) {
	return nil, nil
}
func (s *stubReadModelRepo) SearchItems(ctx context.Context, orgID uuid.UUID, query string) ([]domain.ItemRM, error) {
	return nil, nil
}
func (s *stubReadModelRepo) GetContact(ctx context.Context, id uuid.UUID) (*domain.ContactRM, error) {
	return nil, nil
}
func (s *stubReadModelRepo) GetPrimaryContact(ctx context.Context, customerID uuid.UUID) (*domain.ContactRM, error) {
	return nil, nil
}
func (s *stubReadModelRepo) SearchContacts(ctx context.Context, orgID uuid.UUID, customerID uuid.UUID, query string) ([]domain.ContactRM, error) {
	return nil, nil
}
func (s *stubReadModelRepo) GetOrganization(ctx context.Context, id uuid.UUID) (*domain.OrganizationRM, error) {
	return &domain.OrganizationRM{ID: id, Timezone: "UTC"}, nil
}
func (s *stubReadModelRepo) GetUserByID(ctx context.Context, userID string) (*domain.UserReadOnly, error) {
	return nil, nil
}
func (s *stubReadModelRepo) SearchWorkOrders(ctx context.Context, orgID uuid.UUID, query string) ([]domain.WorkOrderRM, error) {
	return nil, nil
}
func (s *stubReadModelRepo) GetWorkOrderByID(ctx context.Context, id uuid.UUID) (*domain.WorkOrderRM, error) {
	return nil, nil
}
func (s *stubReadModelRepo) GetOrganizationAdminID(ctx context.Context, orgID uuid.UUID) (*uuid.UUID, error) {
	return nil, nil
}
func (s *stubReadModelRepo) GetServiceAppointments(ctx context.Context, workOrderID uuid.UUID) ([]domain.ServiceAppointmentRM, error) {
	return nil, nil
}
func (s *stubReadModelRepo) GetTechnicianNamesForAppointment(ctx context.Context, appointmentID uuid.UUID) ([]string, error) {
	return nil, nil
}
func (s *stubReadModelRepo) GetTechniciansForAppointment(ctx context.Context, appointmentID uuid.UUID) ([]domain.UserReadOnly, error) {
	return nil, nil
}
func (s *stubReadModelRepo) GetCustomerIDsByEmailAndOrg(ctx context.Context, orgID uuid.UUID, email string) ([]uuid.UUID, error) {
	return nil, nil
}
func (s *stubReadModelRepo) GetAddress(ctx context.Context, id uuid.UUID) (*domain.AddressReadOnly, error) {
	return nil, nil
}

func makeHS256JWT(t *testing.T, secret string, claims map[string]interface{}) string {
	t.Helper()
	hdrJSON, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	hdr := base64.RawURLEncoding.EncodeToString(hdrJSON)
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := hdr + "." + payload
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + sig
}

func TestResolveOrganizationID(t *testing.T) {
	headerOrg := uuid.New()
	bodyOrg := uuid.New()
	jwtOrg := uuid.New()
	const secret = "test-secret"
	t.Setenv("JWT_SECRET", secret)

	validToken := makeHS256JWT(t, secret, map[string]interface{}{
		"sub":             uuid.New().String(),
		"organization_id": jwtOrg.String(),
	})

	t.Run("prefers X-Organization-ID header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/billing/sales-orders", nil)
		req.Header.Set("X-Organization-ID", headerOrg.String())
		req.Header.Set("Authorization", "Bearer "+validToken)
		if got := resolveOrganizationID(req, bodyOrg); got != headerOrg {
			t.Fatalf("expected %s, got %s", headerOrg, got)
		}
	})

	t.Run("preserves JSON body fallback when header missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/billing/sales-orders", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		if got := resolveOrganizationID(req, bodyOrg); got != bodyOrg {
			t.Fatalf("expected %s, got %s", bodyOrg, got)
		}
	})

	t.Run("extracts organization_id from verified JWT when header and body missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/billing/sales-orders", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		if got := resolveOrganizationID(req); got != jwtOrg {
			t.Fatalf("expected %s, got %s", jwtOrg, got)
		}
	})

	t.Run("rejects JWT signed with wrong secret", func(t *testing.T) {
		badToken := makeHS256JWT(t, "wrong-secret", map[string]interface{}{
			"organization_id": jwtOrg.String(),
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/billing/sales-orders", nil)
		req.Header.Set("Authorization", "Bearer "+badToken)
		if got := resolveOrganizationID(req); got != uuid.Nil {
			t.Fatalf("expected uuid.Nil, got %s", got)
		}
	})
}

func TestCreateSalesOrder_ResolvesOrganizationID(t *testing.T) {
	const secret = "test-secret"
	t.Setenv("JWT_SECRET", secret)

	customerID := uuid.New()
	itemID := uuid.New()
	now := time.Now().UTC()

	makeBody := func(orgID *uuid.UUID) []byte {
		payload := map[string]interface{}{
			"customer_id": customerID.String(),
			"order_date":  now.Format(time.RFC3339),
			"items": []map[string]interface{}{
				{
					"item_id":    itemID.String(),
					"item_type":  "part",
					"name":       "Chain Sprocket",
					"quantity":   1,
					"unit_price": 1000,
				},
			},
		}
		if orgID != nil {
			payload["organization_id"] = orgID.String()
		}
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		return b
	}

	t.Run("accepts organization_id from JSON payload when X-Organization-ID header absent", func(t *testing.T) {
		repo := &stubSalesOrderRepo{}
		svc := application.NewSalesOrderService(repo, nil, &stubReadModelRepo{}, &stubEventPublisher{}, nil, nil, nil)
		handler := NewSalesOrderHandler(svc)

		orgID := uuid.New()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/billing/sales-orders", bytes.NewReader(makeBody(&orgID)))
		rec := httptest.NewRecorder()

		handler.CreateSalesOrder(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
		}
		if len(repo.created) != 1 || repo.created[0].OrganizationID != orgID {
			t.Fatalf("expected persisted OrganizationID %s, got %+v", orgID, repo.created)
		}
	})

	t.Run("extracts organization_id from Authorization Bearer JWT when header and JSON omit it", func(t *testing.T) {
		repo := &stubSalesOrderRepo{}
		svc := application.NewSalesOrderService(repo, nil, &stubReadModelRepo{}, &stubEventPublisher{}, nil, nil, nil)
		handler := NewSalesOrderHandler(svc)

		orgID := uuid.New()
		token := makeHS256JWT(t, secret, map[string]interface{}{
			"sub":             uuid.New().String(),
			"organization_id": orgID.String(),
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/billing/sales-orders", bytes.NewReader(makeBody(nil)))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		handler.CreateSalesOrder(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
		}
		if len(repo.created) != 1 || repo.created[0].OrganizationID != orgID {
			t.Fatalf("expected persisted OrganizationID %s, got %+v", orgID, repo.created)
		}
	})

	t.Run("returns 400 when organization_id missing from header, payload, and JWT", func(t *testing.T) {
		repo := &stubSalesOrderRepo{}
		svc := application.NewSalesOrderService(repo, nil, &stubReadModelRepo{}, &stubEventPublisher{}, nil, nil, nil)
		handler := NewSalesOrderHandler(svc)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/billing/sales-orders", bytes.NewReader(makeBody(nil)))
		rec := httptest.NewRecorder()

		handler.CreateSalesOrder(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
