package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"erp-billing-service/internal/domain"
	"erp-billing-service/internal/ports/outbound"

	"github.com/google/uuid"
)

type CustomerHTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewCustomerHTTPClient(baseURL string) outbound.CustomerClient {
	return &CustomerHTTPClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func setContextAuthHeaders(ctx context.Context, req *http.Request) {
	if authHeader, ok := ctx.Value("authorization").(string); ok && authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if userID, ok := ctx.Value("user_id").(string); ok && userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	if orgID, ok := ctx.Value("organization_id").(string); ok && orgID != "" {
		req.Header.Set("X-Organization-Id", orgID)
	} else if orgID, ok := ctx.Value("organization_id").(uuid.UUID); ok && orgID != uuid.Nil {
		req.Header.Set("X-Organization-Id", orgID.String())
	}
}

func (c *CustomerHTTPClient) GetCustomer(ctx context.Context, id uuid.UUID) (*domain.CustomerRM, error) {
	url := fmt.Sprintf("%s/api/v1/customers/%s", c.baseURL, id.String())
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	setContextAuthHeaders(ctx, req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // Not found is valid result
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("customer service returned status: %d", resp.StatusCode)
	}

	// Customer Service wraps responses in a "data" field
	var response struct {
		Status string `json:"status"`
		Data   struct {
			ID                uuid.UUID  `json:"id"`
			OrganizationID    uuid.UUID  `json:"organization_id"`
			DisplayName       string     `json:"display_name"`
			CompanyName       string     `json:"company_name"`
			FirstName         string     `json:"first_name"`
			LastName          string     `json:"last_name"`
			Email             string     `json:"email"`
			PhoneWork         string     `json:"phone_work"`
			PhoneMobile       string     `json:"phone_mobile"`
			TaxNumber         string     `json:"tax_number"`
			CurrencyCode      string     `json:"currency_code"`
			PaymentTerms      string     `json:"payment_terms"`
			SourceSystem      string     `json:"source_system"`
			ServiceAddressID  *uuid.UUID `json:"service_address_id"`
			BillingAddressID  *uuid.UUID `json:"billing_address_id"`
			ShippingAddressID *uuid.UUID `json:"shipping_address_id"`
			CreatedAt         time.Time  `json:"created_at"`
			Street1           string     `json:"street1"`
			City              string     `json:"city"`
			State             string     `json:"state"`
			ZipCode           string     `json:"zip_code"`
			Country           string     `json:"country"`
			ShippingStreet1   string     `json:"shipping_street1"`
			ShippingCity      string     `json:"shipping_city"`
			ShippingState     string     `json:"shipping_state"`
			ShippingZipCode   string     `json:"shipping_zip_code"`
			ShippingCountry   string     `json:"shipping_country"`
			Addresses         []struct {
				ID         uuid.UUID `json:"id"`
				Type       string    `json:"type"`
				Street1    string    `json:"street1"`
				Street2    string    `json:"street2"`
				City       string    `json:"city"`
				State      string    `json:"state"`
				PostalCode string    `json:"postal_code"`
				Country    string    `json:"country"`
			} `json:"addresses"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	externalCust := response.Data
	displayName := externalCust.DisplayName
	if displayName == "" {
		displayName = strings.TrimSpace(externalCust.FirstName + " " + externalCust.LastName)
	}
	if displayName == "" {
		displayName = externalCust.CompanyName
	}

	phone := externalCust.PhoneWork
	if phone == "" {
		phone = externalCust.PhoneMobile
	}

	rm := &domain.CustomerRM{
		ID:                externalCust.ID,
		OrganizationID:    externalCust.OrganizationID,
		DisplayName:       displayName,
		CompanyName:       externalCust.CompanyName,
		FirstName:         externalCust.FirstName,
		LastName:          externalCust.LastName,
		Email:             externalCust.Email,
		PhoneWork:         externalCust.PhoneWork,
		PhoneMobile:       externalCust.PhoneMobile,
		Phone:             phone,
		TaxNumber:         externalCust.TaxNumber,
		CurrencyCode:      externalCust.CurrencyCode,
		PaymentTerms:      externalCust.PaymentTerms,
		SourceSystem:      externalCust.SourceSystem,
		ServiceAddressID:  externalCust.ServiceAddressID,
		BillingAddressID:  externalCust.BillingAddressID,
		ShippingAddressID: externalCust.ShippingAddressID,
		CreatedAt:         externalCust.CreatedAt,
		BillingStreet:     externalCust.Street1,
		BillingCity:       externalCust.City,
		BillingState:      externalCust.State,
		BillingCode:       externalCust.ZipCode,
		BillingCountry:    externalCust.Country,
		ShippingStreet:    externalCust.ShippingStreet1,
		ShippingCity:      externalCust.ShippingCity,
		ShippingState:     externalCust.ShippingState,
		ShippingCode:      externalCust.ShippingZipCode,
		ShippingCountry:   externalCust.ShippingCountry,
		UpdatedAt:         time.Now(),
	}

	for _, addr := range externalCust.Addresses {
		street := addr.Street1
		if addr.Street2 != "" {
			if street != "" {
				street += ", " + addr.Street2
			} else {
				street = addr.Street2
			}
		}
		isBilling := (externalCust.BillingAddressID != nil && *externalCust.BillingAddressID == addr.ID) || strings.EqualFold(addr.Type, "billing")
		isShipping := (externalCust.ShippingAddressID != nil && *externalCust.ShippingAddressID == addr.ID) || strings.EqualFold(addr.Type, "shipping")
		if isBilling && rm.BillingStreet == "" {
			rm.BillingStreet = street
			rm.BillingCity = addr.City
			rm.BillingState = addr.State
			rm.BillingCode = addr.PostalCode
			rm.BillingCountry = addr.Country
		}
		if isShipping && rm.ShippingStreet == "" {
			rm.ShippingStreet = street
			rm.ShippingCity = addr.City
			rm.ShippingState = addr.State
			rm.ShippingCode = addr.PostalCode
			rm.ShippingCountry = addr.Country
		}
	}

	return rm, nil
}

func (c *CustomerHTTPClient) GetContact(ctx context.Context, id uuid.UUID) (*domain.ContactRM, error) {
	url := fmt.Sprintf("%s/api/v1/contacts/%s", c.baseURL, id.String())
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	setContextAuthHeaders(ctx, req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("customer service returned status: %d", resp.StatusCode)
	}

	// Contact Service also wraps responses
	var response struct {
		Status string `json:"status"`
		Data   struct {
			ID          uuid.UUID `json:"id"`
			CustomerID  uuid.UUID `json:"customer_id"`
			FirstName   string    `json:"first_name"`
			LastName    string    `json:"last_name"`
			Email       string    `json:"email"`
			Phone       string    `json:"phone"`
			PhoneWork   string    `json:"phone_work"`
			PhoneMobile string    `json:"phone_mobile"`
			Mobile      string    `json:"mobile"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	externalContact := response.Data
	phone := externalContact.Phone
	if phone == "" {
		phone = externalContact.PhoneWork
	}
	if phone == "" {
		phone = externalContact.PhoneMobile
	}
	mobile := externalContact.Mobile
	if mobile == "" {
		mobile = externalContact.PhoneMobile
	}
	return &domain.ContactRM{
		ID:         externalContact.ID,
		CustomerID: externalContact.CustomerID,
		FirstName:  externalContact.FirstName,
		LastName:   externalContact.LastName,
		Email:      externalContact.Email,
		Phone:      phone,
		Mobile:     mobile,
		UpdatedAt:  time.Now(),
	}, nil
}
