package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"erp-billing-service/internal/domain"
	"erp-billing-service/internal/ports/outbound"

	"github.com/google/uuid"
)

// OrganizationHTTPClient fetches organization details over HTTP as a fallback when the read model is absent.
type OrganizationHTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewOrganizationHTTPClient creates a new OrganizationHTTPClient.
func NewOrganizationHTTPClient(baseURL string) outbound.OrganizationClient {
	return &OrganizationHTTPClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *OrganizationHTTPClient) GetOrganization(ctx context.Context, id uuid.UUID) (*domain.OrganizationRM, error) {
	url := fmt.Sprintf("%s/organizations/%s", c.baseURL, id.String())
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if authHeader, ok := ctx.Value("authorization").(string); ok && authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if userID, ok := ctx.Value("user_id").(string); ok && userID != "" {
		req.Header.Set("X-User-ID", userID)
	} else {
		req.Header.Set("X-User-ID", "billing-service")
	}
	if orgID, ok := ctx.Value("organization_id").(string); ok && orgID != "" {
		req.Header.Set("X-Organization-ID", orgID)
	} else {
		req.Header.Set("X-Organization-ID", id.String())
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("organization service returned status: %d", resp.StatusCode)
	}

	var orgResp struct {
		ID               string `json:"id"`
		ProfileID        string `json:"profile_id"`
		OrganizationName string `json:"organization_name"`
		OrganizationType string `json:"organization_type"`
		BusinessCategory string `json:"business_category"`
		Language         string `json:"language"`
		CollectsTax      bool   `json:"collects_tax"`
		GSTIN            string `json:"gstin"`
		Address          string `json:"address"`
		City             string `json:"city"`
		State            string `json:"state"`
		ZipCode          string `json:"zip_code"`
		Country          string `json:"country"`
		Phone            string `json:"phone"`
		Email            string `json:"email"`
		Website          string `json:"website"`
		Currency         string `json:"currency"`
		Timezone         string `json:"timezone"`
		IsActive         bool   `json:"is_active"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&orgResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	parsedID := id
	if p, err := uuid.Parse(orgResp.ID); err == nil {
		parsedID = p
	}

	return &domain.OrganizationRM{
		ID:               parsedID,
		ProfileID:        orgResp.ProfileID,
		OrganizationName: orgResp.OrganizationName,
		OrganizationType: orgResp.OrganizationType,
		BusinessCategory: orgResp.BusinessCategory,
		Language:         orgResp.Language,
		CollectsTax:      orgResp.CollectsTax,
		GSTIN:            orgResp.GSTIN,
		Address:          orgResp.Address,
		City:             orgResp.City,
		State:            orgResp.State,
		ZipCode:          orgResp.ZipCode,
		Country:          orgResp.Country,
		Phone:            orgResp.Phone,
		Email:            orgResp.Email,
		Website:          orgResp.Website,
		Currency:         orgResp.Currency,
		Timezone:         orgResp.Timezone,
		IsActive:         orgResp.IsActive,
		SyncedAt:         time.Now().UTC(),
	}, nil
}
