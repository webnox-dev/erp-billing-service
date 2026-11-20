package outbound

import (
	"context"

	"erp-billing-service/internal/domain"

	"github.com/google/uuid"
)

// OrganizationClient fetches organization profile details from the organization service.
type OrganizationClient interface {
	GetOrganization(ctx context.Context, id uuid.UUID) (*domain.OrganizationRM, error)
}
