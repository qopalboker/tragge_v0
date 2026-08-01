DROP TRIGGER IF EXISTS trg_support_tickets_updated_at ON support_tickets;
DROP FUNCTION IF EXISTS update_support_ticket_timestamp();
DROP TABLE IF EXISTS ticket_attachments;
DROP TABLE IF EXISTS ticket_messages;
DROP TABLE IF EXISTS support_tickets;
DROP TYPE IF EXISTS ticket_priority;
DROP TYPE IF EXISTS ticket_status;
DROP TYPE IF EXISTS ticket_category;
