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

func TestSalesOrderCalculateTotals(t *testing.T) {
	so := &SalesOrder{
		TDSPercentage: 2,
		TCSPercentage: 1,
		Adjustment:    10,
		Items: []SalesOrderItem{
			{Quantity: 2, UnitPrice: 500, Discount: 100, Tax: 162},
		},
	}
	so.CalculateTotals()

	if so.SubTotal != 1000 {
		t.Fatalf("SubTotal = %v, want 1000", so.SubTotal)
	}
	if so.DiscountTotal != 100 {
		t.Fatalf("DiscountTotal = %v, want 100", so.DiscountTotal)
	}
	if so.TaxTotal != 162 {
		t.Fatalf("TaxTotal = %v, want 162", so.TaxTotal)
	}
	if so.TDSAmount != 20 {
		t.Fatalf("TDSAmount = %v, want 20", so.TDSAmount)
	}
	if so.TCSAmount != 10 {
		t.Fatalf("TCSAmount = %v, want 10", so.TCSAmount)
	}
	// 1000 - 100 + 162 - 20 + 10 + 10 = 1062
	if so.TotalAmount != 1062 {
		t.Fatalf("TotalAmount = %v, want 1062", so.TotalAmount)
	}
}
