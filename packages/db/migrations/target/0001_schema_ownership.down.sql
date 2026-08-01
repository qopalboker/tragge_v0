-- Development/test rollback only. Production uses restore or a compensating
-- forward migration after the rollback window closes.
BEGIN;
DROP SCHEMA IF EXISTS market_data CASCADE;
DROP SCHEMA IF EXISTS engine CASCADE;
DROP SCHEMA IF EXISTS platform CASCADE;
COMMIT;
