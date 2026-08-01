-- 0081_manual_kyc_iranian.down.sql
-- Reverse Iranian manual KYC fields and rejection tracking.

-- Drop index first
DROP INDEX IF EXISTS idx_user_verification_national_code_manual;

-- Drop selfie_with_doc_url from kyc_documents
ALTER TABLE kyc_documents
    DROP COLUMN IF EXISTS selfie_with_doc_url;

-- Drop columns from user_verification (reverse order of addition)
ALTER TABLE user_verification
    DROP COLUMN IF EXISTS admin_notes,
    DROP COLUMN IF EXISTS rejection_field_messages,
    DROP COLUMN IF EXISTS rejection_fields,
    DROP COLUMN IF EXISTS birth_certificate_serial,
    DROP COLUMN IF EXISTS birth_certificate_number,
    DROP COLUMN IF EXISTS father_name,
    DROP COLUMN IF EXISTS national_code_manual;

-- NOTE: PostgreSQL does not support removing enum values.
-- The 'birth_certificate' value in kyc_document_type will remain.
