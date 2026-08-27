-- Versioned migration ledger.
-- The runner bootstraps this table before checking applied migrations so
-- existing installations without a ledger can safely record the baseline.
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    text PRIMARY KEY,
    checksum   text NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now()
);
