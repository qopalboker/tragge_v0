-- 0036_email_template_versions.up.sql
-- Multi-version email templates with active/inactive support

-- ============================================================================
-- EMAIL TEMPLATE VERSIONS TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS email_template_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug VARCHAR(100) NOT NULL REFERENCES email_templates(slug) ON DELETE CASCADE,
    version_name VARCHAR(200) NOT NULL,         -- human-readable name, e.g. "Dark Theme v2", "Minimal RTL"
    html_body TEXT NOT NULL,                     -- the HTML body content (the <body> inner content)
    css_content TEXT NOT NULL DEFAULT '',        -- separated CSS styles
    font_config JSONB NOT NULL DEFAULT '{}',    -- per-language font configuration
    is_active BOOLEAN NOT NULL DEFAULT false,   -- only one active per slug
    created_by UUID REFERENCES users(id),
    updated_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Ensure only one active version per slug
CREATE UNIQUE INDEX idx_email_template_versions_active
    ON email_template_versions (slug)
    WHERE is_active = true;

-- Index for listing versions of a slug
CREATE INDEX idx_email_template_versions_slug
    ON email_template_versions (slug, created_at DESC);

-- Enforce max 5 versions per slug via trigger
CREATE OR REPLACE FUNCTION check_max_template_versions()
RETURNS TRIGGER AS $$
BEGIN
    IF (SELECT COUNT(*) FROM email_template_versions WHERE slug = NEW.slug) >= 5 THEN
        RAISE EXCEPTION 'Maximum 5 template versions per slug allowed';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_check_max_template_versions
    BEFORE INSERT ON email_template_versions
    FOR EACH ROW
    EXECUTE FUNCTION check_max_template_versions();

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON TABLE email_template_versions IS 'Multiple template versions per email type. Max 5 per slug, only 1 active.';
COMMENT ON COLUMN email_template_versions.html_body IS 'HTML body content without <style> tags — CSS is stored separately';
COMMENT ON COLUMN email_template_versions.css_content IS 'CSS styles, will be injected into <style> tag when rendering';
COMMENT ON COLUMN email_template_versions.font_config IS 'Per-language font config JSON: {"en": {"family": "Inter", "url": "..."}, "fa": {"family": "Vazirmatn", "url": "..."}}';
COMMENT ON COLUMN email_template_versions.is_active IS 'Only one version per slug can be active. Enforced by partial unique index.';
