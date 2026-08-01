-- 0041_jibit_kyc_fields.down.sql
-- Remove Jibit identity verification fields from user_verification table.

DROP INDEX IF EXISTS idx_user_verification_national_code;

ALTER TABLE user_verification
    DROP COLUMN IF EXISTS national_code,
    DROP COLUMN IF EXISTS phone,
    DROP COLUMN IF EXISTS shahkar_verified,
    DROP COLUMN IF EXISTS face_verified,
    DROP COLUMN IF EXISTS face_match_score,
    DROP COLUMN IF EXISTS liveness_score,
    DROP COLUMN IF EXISTS liveness_result,
    DROP COLUMN IF EXISTS card_ocr_verified,
    DROP COLUMN IF EXISTS card_serial_number,
    DROP COLUMN IF EXISTS jibit_transaction_ids;
