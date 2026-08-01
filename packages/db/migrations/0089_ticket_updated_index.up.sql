CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_support_tickets_updated_at ON support_tickets(updated_at DESC);
