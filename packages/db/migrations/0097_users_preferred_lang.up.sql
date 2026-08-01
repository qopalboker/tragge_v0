ALTER TABLE users ADD COLUMN IF NOT EXISTS preferred_lang VARCHAR(5) NOT NULL DEFAULT 'fa';

COMMENT ON COLUMN users.preferred_lang IS 'Preferred language for emails and notifications: fa (Farsi) or en (English)';
