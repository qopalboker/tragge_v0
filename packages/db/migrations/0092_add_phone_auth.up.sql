-- Add phone number column to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS phone VARCHAR(20) UNIQUE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS phone_verified BOOLEAN NOT NULL DEFAULT false;

-- OTP audit log table for tracking sent codes
CREATE TABLE IF NOT EXISTS otp_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone VARCHAR(20) NOT NULL,
    sent_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    verified_at TIMESTAMP WITH TIME ZONE,
    ip_address VARCHAR(45),
    user_agent TEXT
);

CREATE INDEX IF NOT EXISTS idx_otp_logs_phone ON otp_logs(phone);
CREATE INDEX IF NOT EXISTS idx_otp_logs_sent_at ON otp_logs(sent_at);

-- Make email optional (users can register with phone only)
ALTER TABLE users ALTER COLUMN email DROP NOT NULL;
