-- ==============================================================================
-- PostgreSQL Privilege Grants for Application Tables
-- ==============================================================================
-- This script grants specific privileges to database users after tables
-- have been created by migrations.
--
-- Run this script after running 01-create-users.sql and all migrations.
--
-- Usage:
--   psql -U tragge_admin -d app -f 02-grant-privileges.sql
--
-- ==============================================================================

-- ==============================================================================
-- 1. GRANT APP ROLE PRIVILEGES ON EXISTING TABLES
-- ==============================================================================
-- Note: These grants are wrapped in IF EXISTS checks because this script
-- runs during container initialization, before migrations create the tables.
-- Re-run this script after migrations for full privilege setup.

DO $$
BEGIN
    -- Users and Authentication tables
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'users') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE ON public.users TO tragge_app_role';
        RAISE NOTICE 'Granted privileges on users table';
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'roles') THEN
        EXECUTE 'GRANT SELECT ON public.roles TO tragge_app_role';
        RAISE NOTICE 'Granted privileges on roles table';
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'user_roles') THEN
        EXECUTE 'GRANT SELECT, INSERT, DELETE ON public.user_roles TO tragge_app_role';
        RAISE NOTICE 'Granted privileges on user_roles table';
    END IF;

    -- Contest tables
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'contests') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE ON public.contests TO tragge_app_role';
        RAISE NOTICE 'Granted privileges on contests table';
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'contest_symbols') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON public.contest_symbols TO tragge_app_role';
        RAISE NOTICE 'Granted privileges on contest_symbols table';
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'contest_participants') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE ON public.contest_participants TO tragge_app_role';
        RAISE NOTICE 'Granted privileges on contest_participants table';
    END IF;

    -- Trading tables
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'orders') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE ON public.orders TO tragge_app_role';
        RAISE NOTICE 'Granted privileges on orders table';
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'fills') THEN
        EXECUTE 'GRANT SELECT, INSERT ON public.fills TO tragge_app_role';
        RAISE NOTICE 'Granted privileges on fills table';
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'positions') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE ON public.positions TO tragge_app_role';
        RAISE NOTICE 'Granted privileges on positions table';
    END IF;

    -- Leaderboard and Audit
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'leaderboard_snapshots') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON public.leaderboard_snapshots TO tragge_app_role';
        RAISE NOTICE 'Granted privileges on leaderboard_snapshots table';
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'audit_logs') THEN
        EXECUTE 'GRANT SELECT, INSERT ON public.audit_logs TO tragge_app_role';
        RAISE NOTICE 'Granted privileges on audit_logs table';
    END IF;
END
$$;

-- ==============================================================================
-- 2. GRANT SEQUENCE PRIVILEGES
-- ==============================================================================

-- Grant usage on all sequences for ID generation
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO tragge_app_role;

-- ==============================================================================
-- 3. GRANT READONLY ROLE PRIVILEGES
-- ==============================================================================

-- Readonly gets SELECT on all tables
GRANT SELECT ON ALL TABLES IN SCHEMA public TO tragge_readonly_role;

-- ==============================================================================
-- 4. RESTRICTED TABLES (Admin Only)
-- ==============================================================================

-- Certain sensitive operations should only be done by admin
-- Revoke DELETE on critical tables from app role
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'users') THEN
        EXECUTE 'REVOKE DELETE ON public.users FROM tragge_app_role';
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'contests') THEN
        EXECUTE 'REVOKE DELETE ON public.contests FROM tragge_app_role';
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'orders') THEN
        EXECUTE 'REVOKE DELETE ON public.orders FROM tragge_app_role';
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'fills') THEN
        EXECUTE 'REVOKE DELETE ON public.fills FROM tragge_app_role';
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'positions') THEN
        EXECUTE 'REVOKE DELETE ON public.positions FROM tragge_app_role';
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'audit_logs') THEN
        EXECUTE 'REVOKE DELETE ON public.audit_logs FROM tragge_app_role';
    END IF;
END
$$;

-- ==============================================================================
-- 5. WALLET TABLES (if exists)
-- ==============================================================================

-- Grant privileges on wallet tables if they exist
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'wallets') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE ON public.wallets TO tragge_app_role';
        RAISE NOTICE 'Granted privileges on wallets table';
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'wallet_ledger') THEN
        EXECUTE 'GRANT SELECT, INSERT ON public.wallet_ledger TO tragge_app_role';
        RAISE NOTICE 'Granted privileges on wallet_ledger table';
    END IF;
END
$$;

-- ==============================================================================
-- 6. USER STATS TABLES (if exists)
-- ==============================================================================

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'user_stats') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE ON public.user_stats TO tragge_app_role';
        RAISE NOTICE 'Granted privileges on user_stats table';
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'user_score_history') THEN
        EXECUTE 'GRANT SELECT, INSERT ON public.user_score_history TO tragge_app_role';
        RAISE NOTICE 'Granted privileges on user_score_history table';
    END IF;
END
$$;

-- ==============================================================================
-- 7. ROW LEVEL SECURITY (Optional - for enhanced security)
-- ==============================================================================

-- Example: Enable RLS on users table
-- Users can only see/modify their own data through the app
-- ALTER TABLE public.users ENABLE ROW LEVEL SECURITY;
--
-- CREATE POLICY users_app_policy ON public.users
--     FOR ALL
--     TO tragge_app_role
--     USING (true)  -- App can see all users
--     WITH CHECK (true);  -- App can modify (authentication handled in app layer)

-- ==============================================================================
-- 8. VERIFY PRIVILEGES
-- ==============================================================================

-- Display current privileges for verification
SELECT
    grantee,
    table_schema,
    table_name,
    privilege_type
FROM information_schema.table_privileges
WHERE grantee IN ('tragge_app_role', 'tragge_readonly_role', 'tragge_admin_role')
ORDER BY grantee, table_name, privilege_type;

-- ==============================================================================
-- SUMMARY
-- ==============================================================================
--
-- tragge_app_role privileges:
--   - SELECT, INSERT, UPDATE on most tables
--   - No DELETE on critical tables (users, contests, orders, fills, positions, audit_logs)
--   - USAGE on all sequences
--
-- tragge_readonly_role privileges:
--   - SELECT on all tables
--
-- tragge_admin_role privileges:
--   - ALL on all tables (including DELETE, TRUNCATE, etc.)
--
-- ==============================================================================