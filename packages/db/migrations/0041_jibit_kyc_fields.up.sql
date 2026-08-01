-- 0041_jibit_kyc_fields.up.sql
-- Add Jibit identity verification fields to user_verification table.
-- Supports the 3-step Jibit KYC flow: Shahkar, Face Verification, Card OCR.

ALTER TABLE user_verification
    ADD COLUMN IF NOT EXISTS national_code VARCHAR(10),
    ADD COLUMN IF NOT EXISTS phone VARCHAR(15),
    ADD COLUMN IF NOT EXISTS shahkar_verified BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS face_verified BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS face_match_score DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS liveness_score DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS liveness_result VARCHAR(10),
    ADD COLUMN IF NOT EXISTS card_ocr_verified BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS card_serial_number VARCHAR(50),
    ADD COLUMN IF NOT EXISTS jibit_transaction_ids JSONB NOT NULL DEFAULT '[]';

-- Index on national_code for lookup during verification steps.
CREATE INDEX IF NOT EXISTS idx_user_verification_national_code
    ON user_verification(national_code) WHERE national_code IS NOT NULL;
