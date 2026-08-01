-- 0012_user_status.up.sql
-- Add status column to users table for account activation/suspension

-- Create user status enum
CREATE TYPE user_status AS ENUM ('active', 'suspended', 'pending');

-- Add status column with default 'active'
ALTER TABLE users ADD COLUMN status user_status NOT NULL DEFAULT 'active';

-- Add index for status queries
CREATE INDEX idx_users_status ON users(status);

-- Add composite index for common queries
CREATE INDEX idx_users_email_status ON users(email, status);
