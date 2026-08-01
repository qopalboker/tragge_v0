-- FND-004 target role foundation.
-- Run with a PostgreSQL administrative identity that has CREATEROLE.
-- These are group roles only; login credentials are provisioned outside SQL.

DO $$
DECLARE
    role_name text;
BEGIN
    FOREACH role_name IN ARRAY ARRAY[
        'tragge_migrator',
        'platform_owner',
        'engine_owner',
        'market_data_owner',
        'platform',
        'engine',
        'market_data'
    ]
    LOOP
        IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = role_name) THEN
            EXECUTE format(
                'CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION',
                role_name
            );
        END IF;
    END LOOP;
END
$$;

GRANT platform_owner TO tragge_migrator;
GRANT engine_owner TO tragge_migrator;
GRANT market_data_owner TO tragge_migrator;

ALTER ROLE platform SET search_path = platform, pg_catalog;
ALTER ROLE engine SET search_path = engine, pg_catalog;
ALTER ROLE market_data SET search_path = market_data, pg_catalog;
