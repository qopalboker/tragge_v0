-- Drop trigger
DROP TRIGGER IF EXISTS set_users_updated_at ON users;

-- Drop indexes
DROP INDEX IF EXISTS idx_users_country;
DROP INDEX IF EXISTS idx_users_username;

-- Drop columns
ALTER TABLE users DROP COLUMN IF EXISTS updated_at;
ALTER TABLE users DROP COLUMN IF EXISTS phone;
ALTER TABLE users DROP COLUMN IF EXISTS country;
ALTER TABLE users DROP COLUMN IF EXISTS bio;
ALTER TABLE users DROP COLUMN IF EXISTS avatar_url;
ALTER TABLE users DROP COLUMN IF EXISTS display_name;
ALTER TABLE users DROP COLUMN IF EXISTS username;
