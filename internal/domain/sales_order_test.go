package domain

import (
	"testing"
	"time"
)

func TestSalesOrderPaidCanDeliverAndTransition(t *testing.T) {
	now := time.Now()

	t.Run("paid order without shipped date can transition to delivered", func(t *testing.T) {
		so := &SalesOrder{Status: SalesOrderStatusPaid}
		if !so.CanDeliver() {
			t.Fatalf("expected CanDeliver() to be true for paid order without DeliveredDate")
		}
		if err := so.CanTransitionTo(SalesOrderStatusDelivered); err != nil {
			t.Fatalf("expected paid -> delivered transition to succeed, got error: %v", err)
		}
	})

	t.Run("paid order with shipped date set can transition to delivered", func(t *testing.T) {
		so := &SalesOrder{
			Status:      SalesOrderStatusPaid,
			ShippedDate: &now,
		}
		if !so.CanDeliver() {
			t.Fatalf("expected CanDeliver() to be true for paid order with ShippedDate")
		}
		if err := so.CanTransitionTo(SalesOrderStatusDelivered); err != nil {
			t.Fatalf("expected paid -> delivered transition to succeed, got error: %v", err)
		}
	})

	t.Run("shipped order can transition to delivered", func(t *testing.T) {
		so := &SalesOrder{
			Status:      SalesOrderStatusShipped,
			ShippedDate: &now,
		}
		if !so.CanDeliver() {
			t.Fatalf("expected CanDeliver() to be true for shipped order")
		}
		if err := so.CanTransitionTo(SalesOrderStatusDelivered); err != nil {
			t.Fatalf("expected shipped -> delivered transition to succeed, got error: %v", err)
		}
	})

	t.Run("already delivered order cannot deliver again", func(t *testing.T) {
		so := &SalesOrder{
			Status:        SalesOrderStatusDelivered,
			ShippedDate:   &now,
			DeliveredDate: &now,
		}
		if so.CanDeliver() {
			t.Fatalf("expected CanDeliver() to be false when DeliveredDate is already set")
		}
	})
}

func TestNormalizeSalesOrderSourceSystem(t *testing.T) {
	cases := []struct {
		input string
		want  SourceSystem
	}{
		{"crm", SourceSystemCRM},
		{"CRM", SourceSystemCRM},
		{"ims", SourceSystemInventory},
		{"inventory", SourceSystemInventory},
		{"INVENTORY", SourceSystemInventory},
		{"efs", SourceSystemFSM},
		{"fsm", SourceSystemFSM},
		{"", ""},
	}
	for _, tc := range cases {
		if got := NormalizeSalesOrderSourceSystem(tc.input); got != tc.want {
			t.Fatalf("NormalizeSalesOrderSourceSystem(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
