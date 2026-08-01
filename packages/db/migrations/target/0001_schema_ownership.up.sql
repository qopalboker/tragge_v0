BEGIN;

CREATE SCHEMA IF NOT EXISTS platform AUTHORIZATION platform_owner;
CREATE SCHEMA IF NOT EXISTS engine AUTHORIZATION engine_owner;
CREATE SCHEMA IF NOT EXISTS market_data AUTHORIZATION market_data_owner;

REVOKE ALL ON SCHEMA public FROM PUBLIC;
REVOKE ALL ON SCHEMA platform FROM PUBLIC;
REVOKE ALL ON SCHEMA engine FROM PUBLIC;
REVOKE ALL ON SCHEMA market_data FROM PUBLIC;

GRANT USAGE ON SCHEMA platform TO platform;
GRANT USAGE ON SCHEMA engine TO engine;
GRANT USAGE ON SCHEMA market_data TO market_data;

REVOKE ALL ON SCHEMA engine, market_data FROM platform;
REVOKE ALL ON SCHEMA platform, market_data FROM engine;
REVOKE ALL ON SCHEMA platform, engine FROM market_data;

ALTER DEFAULT PRIVILEGES FOR ROLE platform_owner IN SCHEMA platform
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO platform;
ALTER DEFAULT PRIVILEGES FOR ROLE platform_owner IN SCHEMA platform
    GRANT USAGE, SELECT ON SEQUENCES TO platform;

ALTER DEFAULT PRIVILEGES FOR ROLE engine_owner IN SCHEMA engine
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO engine;
ALTER DEFAULT PRIVILEGES FOR ROLE engine_owner IN SCHEMA engine
    GRANT USAGE, SELECT ON SEQUENCES TO engine;

ALTER DEFAULT PRIVILEGES FOR ROLE market_data_owner IN SCHEMA market_data
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO market_data;
ALTER DEFAULT PRIVILEGES FOR ROLE market_data_owner IN SCHEMA market_data
    GRANT USAGE, SELECT ON SEQUENCES TO market_data;

COMMENT ON SCHEMA platform IS
    'Platform modular monolith owned state; no cross-system SQL';
COMMENT ON SCHEMA engine IS
    'Trading Engine owned state; no cross-system SQL';
COMMENT ON SCHEMA market_data IS
    'Market Data Service owned state; no cross-system SQL';

COMMIT;
