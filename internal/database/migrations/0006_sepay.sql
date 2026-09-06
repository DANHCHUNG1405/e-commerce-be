ALTER TABLE payments ADD COLUMN code text UNIQUE;
ALTER TABLE payments ADD COLUMN bank text NOT NULL DEFAULT '';
ALTER TABLE payments ADD COLUMN account_number text NOT NULL DEFAULT '';
ALTER TABLE payments ADD CONSTRAINT payments_sepay_details CHECK (
    method <> 'sepay' OR (code IS NOT NULL AND code ~ '^EC[0-9A-F]{32}$' AND bank <> '' AND account_number <> '' AND amount > 0)
);

-- Durable inbox for duplicates, unknown references, and reconciliation exceptions.
-- No raw body or signature is persisted.
CREATE TABLE payment_webhook_receipts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider text NOT NULL,
    provider_transaction_id text NOT NULL,
    payment_id uuid REFERENCES payments(id),
    code text NOT NULL DEFAULT '',
    bank text NOT NULL,
    account_number text NOT NULL,
    direction text NOT NULL CHECK (direction IN ('in', 'out')),
    amount bigint NOT NULL CHECK (amount >= 0),
    status text NOT NULL CHECK (status IN ('unmatched', 'ignored', 'requires_review', 'applied')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    UNIQUE (provider, provider_transaction_id)
);
CREATE INDEX payment_webhook_review_idx ON payment_webhook_receipts (created_at) WHERE status IN ('unmatched','requires_review');
