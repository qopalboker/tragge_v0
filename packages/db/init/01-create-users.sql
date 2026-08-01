-- ==============================================================================
-- PostgreSQL User Creation and Privilege Management
-- ==============================================================================
-- This script creates database users with role-based access control.
-- Run this as a superuser (postgres) before running application migrations.
--
-- Users created:
--   tragge_admin     - Full administrative access (migrations, DDL)
--   tragge_app       - Application user (CRUD on application tables)
--   tragge_readonly  - Read-only access (replicas, reporting)
--   tragge_replication - Streaming replication (optional)
--   pgbouncer_auth   - PgBouncer authentication lookup
--
-- Environment Variables Required:
--   POSTGRES_ADMIN_PASSWORD
--   POSTGRES_APP_PASSWORD
--   POSTGRES_READONLY_PASSWORD
--   POSTGRES_REPLICATION_PASSWORD (optional)
--   PGBOUNCER_AUTH_PASSWORD
--
-- Usage:
--   psql -U postgres -d app -f 01-create-users.sql
--
-- ==============================================================================

-- Enable pgcrypto for secure password handling
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ==============================================================================
-- 1. CREATE ROLES (without login, for permission grouping)
-- ==============================================================================

-- Role for administrative operations (DDL, migrations)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tragge_admin_role') THEN
        CREATE ROLE tragge_admin_role NOLOGIN;
        RAISE NOTICE 'Created role: tragge_admin_role';
    END IF;
END
$$;

-- Role for application operations (DML on app tables)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tragge_app_role') THEN
        CREATE ROLE tragge_app_role NOLOGIN;
        RAISE NOTICE 'Created role: tragge_app_role';
    END IF;
END
$$;

-- Role for read-only operations
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tragge_readonly_role') THEN
        CREATE ROLE tragge_readonly_role NOLOGIN;
        RAISE NOTICE 'Created role: tragge_readonly_role';
    END IF;
END
$$;

-- ==============================================================================
-- 2. CREATE USERS (with login capability)
-- ==============================================================================

-- Admin user (full access for migrations and maintenance)
-- Password will be set via ALTER USER after creation
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tragge_admin') THEN
        CREATE USER tragge_admin WITH
            LOGIN
            NOSUPERUSER
            CREATEDB
            CREATEROLE
            INHERIT
            CONNECTION LIMIT 10;
        RAISE NOTICE 'Created user: tragge_admin';
    END IF;
END
$$;

-- Application user (limited to necessary operations)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tragge_app') THEN
        CREATE USER tragge_app WITH
            LOGIN
            NOSUPERUSER
            NOCREATEDB
            NOCREATEROLE
            INHERIT
            CONNECTION LIMIT 100;
        RAISE NOTICE 'Created user: tragge_app';
    END IF;
END
$$;

-- Read-only user (for replicas and reporting)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tragge_readonly') THEN
        CREATE USER tragge_readonly WITH
            LOGIN
            NOSUPERUSER
            NOCREATEDB
            NOCREATEROLE
            INHERIT
            CONNECTION LIMIT 50;
        RAISE NOTICE 'Created user: tragge_readonly';
    END IF;
END
$$;

-- Replication user (for streaming replication)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tragge_replication') THEN
        CREATE USER tragge_replication WITH
            LOGIN
            REPLICATION
            NOSUPERUSER
            NOCREATEDB
            NOCREATEROLE
            CONNECTION LIMIT 5;
        RAISE NOTICE 'Created user: tragge_replication';
    END IF;
END
$$;

-- PgBouncer auth user (for userlist lookup)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'pgbouncer_auth') THEN
        CREATE USER pgbouncer_auth WITH
            LOGIN
            NOSUPERUSER
            NOCREATEDB
            NOCREATEROLE
            CONNECTION LIMIT 5;
        RAISE NOTICE 'Created user: pgbouncer_auth';
    END IF;
END
$$;

-- ==============================================================================
-- 3. ASSIGN ROLES TO USERS
-- ==============================================================================

GRANT tragge_admin_role TO tragge_admin;
GRANT tragge_app_role TO tragge_app;
GRANT tragge_readonly_role TO tragge_readonly;

-- ==============================================================================
-- 4. GRANT DATABASE PRIVILEGES
-- ==============================================================================

-- Admin: full database access
GRANT ALL PRIVILEGES ON DATABASE app TO tragge_admin;

-- App: connect to database
GRANT CONNECT ON DATABASE app TO tragge_app;
GRANT CONNECT ON DATABASE app TO tragge_readonly;
GRANT CONNECT ON DATABASE app TO pgbouncer_auth;

-- ==============================================================================
-- 5. GRANT SCHEMA PRIVILEGES
-- ==============================================================================

-- Admin: full schema access including DDL
GRANT ALL PRIVILEGES ON SCHEMA public TO tragge_admin_role;
GRANT CREATE ON SCHEMA public TO tragge_admin_role;

-- App: usage only (no DDL)
GRANT USAGE ON SCHEMA public TO tragge_app_role;
GRANT USAGE ON SCHEMA public TO tragge_readonly_role;

-- ==============================================================================
-- 6. GRANT TABLE PRIVILEGES
-- ==============================================================================

-- Admin: all privileges on all tables
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO tragge_admin_role;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO tragge_admin_role;

-- App: SELECT, INSERT, UPDATE, DELETE on specific tables
-- Note: These will be granted after tables are created in migrations

-- Readonly: SELECT only
GRANT SELECT ON ALL TABLES IN SCHEMA public TO tragge_readonly_role;

-- ==============================================================================
-- 7. SET DEFAULT PRIVILEGES (for future objects)
-- ==============================================================================

-- When tragge_admin creates tables, grant appropriate access to roles
ALTER DEFAULT PRIVILEGES FOR ROLE tragge_admin IN SCHEMA public
    GRANT ALL PRIVILEGES ON TABLES TO tragge_admin_role;

ALTER DEFAULT PRIVILEGES FOR ROLE tragge_admin IN SCHEMA public
    GRANT ALL PRIVILEGES ON SEQUENCES TO tragge_admin_role;

-- App role gets DML access to tables created by admin
ALTER DEFAULT PRIVILEGES FOR ROLE tragge_admin IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO tragge_app_role;

ALTER DEFAULT PRIVILEGES FOR ROLE tragge_admin IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO tragge_app_role;

-- Readonly role gets SELECT on tables created by admin
ALTER DEFAULT PRIVILEGES FOR ROLE tragge_admin IN SCHEMA public
    GRANT SELECT ON TABLES TO tragge_readonly_role;

-- ==============================================================================
-- 8. CREATE PGBOUNCER AUTH FUNCTION
-- ==============================================================================

-- Function for PgBouncer to look up user passwords
-- This allows PgBouncer to authenticate users against PostgreSQL
CREATE OR REPLACE FUNCTION public.pgbouncer_get_auth(p_username TEXT)
RETURNS TABLE(username TEXT, password TEXT) AS
$$
BEGIN
    RETURN QUERY
    SELECT
        usename::TEXT,
        passwd::TEXT
    FROM pg_catalog.pg_shadow
    WHERE usename = p_username;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Revoke access from public, grant only to pgbouncer_auth
REVOKE ALL ON FUNCTION public.pgbouncer_get_auth(TEXT) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.pgbouncer_get_auth(TEXT) TO pgbouncer_auth;

-- ==============================================================================
-- 9. AUDIT LOGGING FOR PRIVILEGE CHANGES
-- ==============================================================================

-- Create audit table for tracking privilege changes (optional)
CREATE TABLE IF NOT EXISTS public.privilege_audit_log (
    id SERIAL PRIMARY KEY,
    event_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    username TEXT NOT NULL,
    action TEXT NOT NULL,
    details JSONB
);

-- Grant insert to admin for logging
GRANT INSERT ON public.privilege_audit_log TO tragge_admin_role;
GRANT SELECT ON public.privilege_audit_log TO tragge_admin_role;
GRANT USAGE ON SEQUENCE public.privilege_audit_log_id_seq TO tragge_admin_role;

-- ==============================================================================
-- 10. SECURITY CONFIGURATIONS
-- ==============================================================================

-- Revoke public schema creation from PUBLIC
REVOKE CREATE ON SCHEMA public FROM PUBLIC;

-- Ensure only admin can create tables
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO PUBLIC;

-- ==============================================================================
-- SUMMARY
-- ==============================================================================
--
-- Users created:
--   tragge_admin      - For migrations, DDL, maintenance (conn limit: 10)
--   tragge_app        - For application services (conn limit: 100)
--   tragge_readonly   - For read replicas, reporting (conn limit: 50)
--   tragge_replication - For streaming replication (conn limit: 5)
--   pgbouncer_auth    - For PgBouncer auth lookup (conn limit: 5)
--
-- To set passwords, run:
--   ALTER USER tragge_admin WITH PASSWORD 'your-secure-password';
--   ALTER USER tragge_app WITH PASSWORD 'your-secure-password';
--   ALTER USER tragge_readonly WITH PASSWORD 'your-secure-password';
--   ALTER USER tragge_replication WITH PASSWORD 'your-secure-password';
--   ALTER USER pgbouncer_auth WITH PASSWORD 'your-secure-password';
--
-- ==============================================================================
