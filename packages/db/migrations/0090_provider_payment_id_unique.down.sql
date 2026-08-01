-- 0090_provider_payment_id_unique.down.sql
-- Revert to non-unique index

DROP INDEX IF EXISTS idx_payment_intents_provider_payment_id;

CREATE INDEX idx_payment_intents_provider_payment_id
ON payment_intents(provider, provider_payment_id);
