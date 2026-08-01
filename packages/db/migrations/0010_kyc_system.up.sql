-- 0010_kyc_system.up.sql
-- KYC (Know Your Customer) verification system

-- ============================================================================
-- ENUMS
-- ============================================================================

CREATE TYPE kyc_status AS ENUM (
    'none',
    'pending',
    'under_review',
    'verified',
    'rejected',
    'expired'
);

CREATE TYPE kyc_document_type AS ENUM (
    'passport',
    'national_id',
    'drivers_license',
    'residence_permit'
);

-- ============================================================================
-- USER VERIFICATION
-- ============================================================================

CREATE TABLE user_verification (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    status kyc_status NOT NULL DEFAULT 'none',
    first_name VARCHAR(100),
    last_name VARCHAR(100),
    date_of_birth DATE,
    nationality VARCHAR(2), -- ISO 3166-1 alpha-2 country code
    address_line1 VARCHAR(255),
    address_line2 VARCHAR(255),
    city VARCHAR(100),
    state VARCHAR(100),
    postal_code VARCHAR(100),
    country VARCHAR(2), -- ISO 3166-1 alpha-2 country code
    verified_at TIMESTAMPTZ,
    verified_by UUID REFERENCES users(id) ON DELETE SET NULL,
    rejection_reason TEXT,
    expires_at TIMESTAMPTZ,
    provider VARCHAR(50), -- external KYC service provider name
    provider_verification_id VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_user_verification_status ON user_verification(status);
CREATE INDEX idx_user_verification_verified_by ON user_verification(verified_by);
CREATE INDEX idx_user_verification_expires_at ON user_verification(expires_at);
CREATE INDEX idx_user_verification_provider ON user_verification(provider);
CREATE INDEX idx_user_verification_created_at ON user_verification(created_at);

-- ============================================================================
-- KYC DOCUMENTS
-- ============================================================================

CREATE TABLE kyc_documents (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    document_type kyc_document_type NOT NULL,
    document_number VARCHAR(100),
    issuing_country VARCHAR(2), -- ISO 3166-1 alpha-2 country code
    issue_date DATE,
    expiry_date DATE,
    front_image_url TEXT,
    back_image_url TEXT,
    selfie_url TEXT,
    status kyc_status NOT NULL DEFAULT 'pending',
    reviewed_at TIMESTAMPTZ,
    reviewed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    review_notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_kyc_documents_user_id ON kyc_documents(user_id);
CREATE INDEX idx_kyc_documents_status ON kyc_documents(status);
CREATE INDEX idx_kyc_documents_document_type ON kyc_documents(document_type);
CREATE INDEX idx_kyc_documents_reviewed_by ON kyc_documents(reviewed_by);
CREATE INDEX idx_kyc_documents_created_at ON kyc_documents(created_at);
CREATE INDEX idx_kyc_documents_user_status ON kyc_documents(user_id, status);

-- ============================================================================
-- KYC AUDIT LOG
-- ============================================================================

CREATE TABLE kyc_audit_log (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    action VARCHAR(50) NOT NULL, -- 'submitted', 'approved', 'rejected', 'expired', 'resubmitted'
    actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    details JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_kyc_audit_log_user_id ON kyc_audit_log(user_id);
CREATE INDEX idx_kyc_audit_log_action ON kyc_audit_log(action);
CREATE INDEX idx_kyc_audit_log_actor_id ON kyc_audit_log(actor_id);
CREATE INDEX idx_kyc_audit_log_created_at ON kyc_audit_log(created_at);
CREATE INDEX idx_kyc_audit_log_user_action ON kyc_audit_log(user_id, action);

-- ============================================================================
-- TRIGGERS FOR UPDATED_AT
-- ============================================================================

CREATE TRIGGER set_user_verification_updated_at
    BEFORE UPDATE ON user_verification
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();
