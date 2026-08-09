-- SEC-007: Admin-only Super Admin MFA state. The legacy shared-user TOTP
-- columns from migration 0050 remain historical and are not trusted here.

CREATE TABLE admin_mfa_credentials (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    secret_ciphertext TEXT NOT NULL CHECK (secret_ciphertext LIKE 'enc:admin-mfa:v1:%'),
    last_totp_counter BIGINT,
    enabled_at TIMESTAMPTZ NOT NULL,
    recovery_generation INTEGER NOT NULL DEFAULT 1 CHECK (recovery_generation > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE admin_mfa_recovery_codes (
    user_id UUID NOT NULL REFERENCES admin_mfa_credentials(user_id) ON DELETE CASCADE,
    generation INTEGER NOT NULL CHECK (generation > 0),
    code_digest BYTEA NOT NULL CHECK (octet_length(code_digest) = 32),
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, generation, code_digest)
);

CREATE INDEX idx_admin_mfa_recovery_codes_unused
    ON admin_mfa_recovery_codes (user_id, generation)
    WHERE used_at IS NULL;

COMMENT ON TABLE admin_mfa_credentials IS
    'Platform-owned Admin-only Super Admin MFA credentials; never used for User authentication.';
COMMENT ON COLUMN admin_mfa_credentials.last_totp_counter IS
    'Highest accepted RFC 6238 counter, updated atomically to prevent replay.';
COMMENT ON TABLE admin_mfa_recovery_codes IS
    'Keyed digests of single-use Super Admin recovery codes; plaintext is never stored.';
