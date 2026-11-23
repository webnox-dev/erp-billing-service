package domain

import "testing"

func TestInvoiceEffectiveBalanceAndCanReceivePayment(t *testing.T) {
	t.Run("recovers unpaid balance when legacy sent invoice has zero BalanceAmount", func(t *testing.T) {
		inv := &Invoice{
			Status:        InvoiceStatusSent,
			TotalAmount:   2000,
			PaidAmount:    0,
			BalanceAmount: 0,
		}
		if got := inv.EffectiveBalance(); got != 2000 {
			t.Fatalf("expected EffectiveBalance 2000, got %.2f", got)
		}
		if !inv.CanReceivePayment() {
			t.Fatalf("expected CanReceivePayment() to be true when recovered balance is 2000")
		}
	})

	t.Run("uses partial remaining balance when partially paid", func(t *testing.T) {
		inv := &Invoice{
			Status:        InvoiceStatusPartial,
			TotalAmount:   2000,
			PaidAmount:    500,
			BalanceAmount: 0,
		}
		if got := inv.EffectiveBalance(); got != 1500 {
			t.Fatalf("expected EffectiveBalance 1500, got %.2f", got)
		}
		if !inv.CanReceivePayment() {
			t.Fatalf("expected CanReceivePayment() to be true for partially paid invoice")
		}
	})

	t.Run("partial child invoices do not report recoverable unpaid balance", func(t *testing.T) {
		inv := &Invoice{
			Status:           InvoiceStatusPaid,
			IsPartialInvoice: true,
			TotalAmount:      500,
			PaidAmount:       500,
			BalanceAmount:    0,
		}
		if got := inv.EffectiveBalance(); got != 0 {
			t.Fatalf("expected EffectiveBalance 0 for partial invoice, got %.2f", got)
		}
		if inv.CanReceivePayment() {
			t.Fatalf("expected CanReceivePayment() to be false for partial invoice")
		}
	})
}
