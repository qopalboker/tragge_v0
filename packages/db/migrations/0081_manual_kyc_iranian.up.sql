-- 0081_manual_kyc_iranian.up.sql
-- Add Iranian manual KYC fields and per-field rejection tracking.

-- ============================================================================
-- USER VERIFICATION: Iranian KYC fields + rejection detail tracking
-- ============================================================================

ALTER TABLE user_verification
    ADD COLUMN IF NOT EXISTS national_code_manual VARCHAR(10),
    ADD COLUMN IF NOT EXISTS father_name VARCHAR(100),
    ADD COLUMN IF NOT EXISTS birth_certificate_number VARCHAR(20),
    ADD COLUMN IF NOT EXISTS birth_certificate_serial VARCHAR(30),
    ADD COLUMN IF NOT EXISTS rejection_fields JSONB DEFAULT '[]',
    ADD COLUMN IF NOT EXISTS rejection_field_messages JSONB DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS admin_notes TEXT;

-- ============================================================================
-- KYC DOCUMENTS: selfie with document image
-- ============================================================================

ALTER TABLE kyc_documents
    ADD COLUMN IF NOT EXISTS selfie_with_doc_url TEXT;

-- ============================================================================
-- KYC DOCUMENT TYPE: add birth_certificate
-- ============================================================================

-- Add birth_certificate to kyc_document_type enum if it doesn't exist
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_enum WHERE enumlabel = 'birth_certificate' AND enumtypid = 'kyc_document_type'::regtype) THEN
        ALTER TYPE kyc_document_type ADD VALUE 'birth_certificate';
    END IF;
END
$$;

-- ============================================================================
-- INDEXES
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_user_verification_national_code_manual
    ON user_verification(national_code_manual) WHERE national_code_manual IS NOT NULL;
