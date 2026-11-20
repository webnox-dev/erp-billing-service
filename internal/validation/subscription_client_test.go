package validation

import (
	"strings"
	"testing"
)

func TestNormalizeBillingModule(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  string
	}{
		{name: "crm lowercase", input: []string{"crm"}, want: "crm"},
		{name: "crm uppercase trimmed", input: []string{" CRM "}, want: "crm"},
		{name: "inventory maps to ims", input: []string{"inventory"}, want: "ims"},
		{name: "ims maps to ims", input: []string{"ims"}, want: "ims"},
		{name: "billing maps to efs", input: []string{"billing"}, want: "efs"},
		{name: "fsm maps to efs", input: []string{"fsm"}, want: "efs"},
		{name: "efs maps to efs", input: []string{"efs"}, want: "efs"},
		{name: "skips unknown then matches", input: []string{"", "unknown", "crm"}, want: "crm"},
		{name: "empty returns empty", input: nil, want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeBillingModule(tc.input); got != tc.want {
				t.Fatalf("normalizeBillingModule(%v) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestFormatBillingRestrictionMessage(t *testing.T) {
	crmMsg := formatBillingRestrictionMessage(RestrictionKeyMaxSalesOrders, "crm", 3, 1)
	if !strings.Contains(crmMsg, "purchased CRM plan") || !strings.Contains(crmMsg, "(3/1)") {
		t.Fatalf("unexpected CRM message: %q", crmMsg)
	}

	imsMsg := formatBillingRestrictionMessage(RestrictionKeyMaxSalesOrders, "ims", 10, 10)
	if !strings.Contains(imsMsg, "purchased IMS plan") || !strings.Contains(imsMsg, "(10/10)") {
		t.Fatalf("unexpected IMS message: %q", imsMsg)
	}

	genericMsg := formatBillingRestrictionMessage(RestrictionKeyMaxSalesOrders, "", 2, 1)
	if !strings.Contains(genericMsg, "your purchased plan") {
		t.Fatalf("unexpected generic message: %q", genericMsg)
	}
}
