-- Migration 006: Add soft-delete support to invoice child tables
-- Adds deleted_at column to invoice_items, payments, and invoice_audit_logs
-- so that all invoice-related records are soft-deleted (deleted_at set)
-- instead of being permanently removed from the database.
--
-- After this migration, GORM's automatic soft-delete behaviour will apply
-- (queries automatically filter WHERE deleted_at IS NULL).

-- ── invoice_items ────────────────────────────────────────────────────────────
ALTER TABLE invoice_items
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_invoice_items_deleted_at
    ON invoice_items (deleted_at);

-- ── payments ─────────────────────────────────────────────────────────────────
ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_payments_deleted_at
    ON payments (deleted_at);

-- ── invoice_audit_logs ───────────────────────────────────────────────────────
ALTER TABLE invoice_audit_logs
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_invoice_audit_logs_deleted_at
    ON invoice_audit_logs (deleted_at);
