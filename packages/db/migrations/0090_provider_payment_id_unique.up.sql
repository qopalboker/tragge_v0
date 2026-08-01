-- 0090_provider_payment_id_unique.up.sql
-- Add UNIQUE constraint on provider_payment_id per provider
-- Prevents duplicate payment intent entries for the same provider payment

-- First, drop the existing non-unique index
DROP INDEX IF EXISTS idx_payment_intents_provider_payment_id;

-- Create a UNIQUE partial index (only for non-NULL values)
CREATE UNIQUE INDEX idx_payment_intents_provider_payment_id
ON payment_intents(provider, provider_payment_id)
WHERE provider_payment_id IS NOT NULL;
