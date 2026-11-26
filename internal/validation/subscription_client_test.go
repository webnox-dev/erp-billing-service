package validation

import (
	"strings"
	"testing"
	"time"
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

func TestBillingIsSubscriptionExpired_EnforcesTrialLimits(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

	if isSubscriptionExpired("sub-trial", "trial", now.Add(48*time.Hour), now) {
		t.Fatalf("active trial within expiry must not be marked expired (and must still enforce trial plan restrictions)")
	}
	if !isSubscriptionExpired("sub-trial", "trial", now.Add(-1*time.Minute), now) {
		t.Fatalf("past-due trial must be marked expired")
	}
	if !isSubscriptionExpired("", "active", now.Add(24*time.Hour), now) {
		t.Fatalf("missing subscription must fall back as expired")
	}
}

func TestPartitionActivePlanItems_PurchasedVsDefaultAndExpiredTrial(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	items := []billingPlanItemCandidate{
		{PlanID: "default-free-ims", IsDefault: true, Price: 0, ValidDays: 0, CreatedAt: now.AddDate(0, 0, -30)},
		{PlanID: "expired-trial-ims", IsDefault: false, Price: 0, ValidDays: 14, CreatedAt: now.AddDate(0, 0, -15)},
		{PlanID: "purchased-std-ims", IsDefault: false, Price: 1999, ValidDays: 30, CreatedAt: now.AddDate(0, 0, -2)},
	}

	nonDefault, all := partitionActivePlanItems(items, now)
	if len(nonDefault) != 1 || nonDefault[0] != "purchased-std-ims" {
		t.Fatalf("nonDefault = %v, want [purchased-std-ims]", nonDefault)
	}
	if len(all) != 2 || all[0] != "default-free-ims" || all[1] != "purchased-std-ims" {
		t.Fatalf("all = %v, want [default-free-ims purchased-std-ims]", all)
	}
}

func TestPickHighestRestrictionLimit(t *testing.T) {
	tests := []struct {
		name      string
		values    []int
		wantLimit int
		wantFound bool
	}{
		{
			name:      "unlimited -1 overrides positive caps",
			values:    []int{5, 100, -1, 50},
			wantLimit: -1,
			wantFound: true,
		},
		{
			name:      "highest numeric cap wins over lower tier cap",
			values:    []int{5, 250, 50},
			wantLimit: 250,
			wantFound: true,
		},
		{
			name:      "empty slice returns not found",
			values:    nil,
			wantLimit: 0,
			wantFound: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, found := pickHighestRestrictionLimit(tc.values)
			if found != tc.wantFound || got != tc.wantLimit {
				t.Fatalf("pickHighestRestrictionLimit(%v) = (%d, %v), want (%d, %v)", tc.values, got, found, tc.wantLimit, tc.wantFound)
			}
		})
	}
}

func TestValidateCustomerEligibleForModule_FailOpenWithoutDB(t *testing.T) {
	client := &SubscriptionClient{}
	allowed, msg, err := client.ValidateCustomerEligibleForModule("org-1", "cust-1", "ims")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatalf("expected allowed=true without db, got false (msg=%q)", msg)
	}
	if !strings.Contains(billingCustomerPriorityOrderClause("ims"), "INVENTORY") {
		t.Fatalf("expected IMS customer priority clause to prioritize INVENTORY")
	}
}
