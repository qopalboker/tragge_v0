-- Baseline down migration: drop all tables and recreate empty schema.
-- WARNING: This destroys all data. Only use for fresh environments.
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO public;
