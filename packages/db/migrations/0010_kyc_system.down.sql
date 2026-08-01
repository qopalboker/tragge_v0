-- 0010_kyc_system.down.sql
-- Rollback KYC verification system

-- Drop trigger first
DROP TRIGGER IF EXISTS set_user_verification_updated_at ON user_verification;

-- Drop tables in reverse order of creation (respecting foreign key dependencies)
DROP TABLE IF EXISTS kyc_audit_log;
DROP TABLE IF EXISTS kyc_documents;
DROP TABLE IF EXISTS user_verification;

-- Drop enums
DROP TYPE IF EXISTS kyc_document_type;
DROP TYPE IF EXISTS kyc_status;
