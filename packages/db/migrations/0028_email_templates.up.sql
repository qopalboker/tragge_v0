-- 0028_email_templates.up.sql
-- Email template overrides for admin customization

-- ============================================================================
-- EMAIL TEMPLATES TABLE
-- ============================================================================

-- Create email_templates table for storing custom template overrides
CREATE TABLE IF NOT EXISTS email_templates (
    slug VARCHAR(100) PRIMARY KEY,           -- matches the template name e.g., 'welcome', 'password_reset'
    subject VARCHAR(500),                    -- optional custom subject line
    html_content TEXT,                       -- the full HTML template content (NULL or empty = use default)
    description VARCHAR(500),                -- human-readable description of what this template is for
    variables TEXT,                          -- comma-separated list of available template variables
    updated_by UUID REFERENCES users(id),    -- last admin who updated the template
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================================
-- INDEXES
-- ============================================================================

-- Index for finding templates by update time (for sorting in admin UI)
CREATE INDEX IF NOT EXISTS idx_email_templates_updated_at ON email_templates(updated_at DESC);

-- ============================================================================
-- SEED DATA: Register all existing templates
-- ============================================================================

-- Insert metadata rows for all existing templates
-- html_content is NULL to indicate "use embedded default"
INSERT INTO email_templates (slug, description, variables) VALUES
    ('welcome', 'Welcome email sent after registration', 'UserEmail, VerificationURL, DashboardURL, Lang'),
    ('email_verification', 'Email verification link', 'UserName, VerificationURL'),
    ('password_reset', 'Password reset link', 'UserName, ResetURL'),
    ('kyc_approved', 'KYC approval notification', 'UserName, ExpiresAt, DashboardURL'),
    ('kyc_rejected', 'KYC rejection notification', 'UserName, Reason, VerificationURL'),
    ('kyc_info_request', 'KYC additional info request', 'UserName, Message, VerificationURL'),
    ('deposit_confirmed', 'Deposit confirmation', 'UserName, Amount, NewBalance, Date, TransactionID, WalletURL'),
    ('withdrawal_approved', 'Withdrawal approved', 'UserName, Amount, AdminComment, DashboardURL'),
    ('withdrawal_rejected', 'Withdrawal rejected', 'UserName, Amount, Reason, DashboardURL'),
    ('withdrawal_processing', 'Withdrawal processing', 'UserName, Amount, DashboardURL'),
    ('withdrawal_completed', 'Withdrawal completed', 'UserName, Amount, DashboardURL'),
    ('contest_starting', 'Contest starting reminder', 'ContestID, ContestName, StartTime, EndTime, Duration, TimeUntilStart, StartingBalance, ParticipantCount, Symbols, TradingURL'),
    ('contest_cancelled', 'Contest cancellation notice', 'UserName, ContestID, ContestName, Reason, ScheduledStart, RefundAmount, NewBalance, ContestsURL'),
    ('contest_summary', 'Contest results summary', 'ContestID, ContestName, Status, StartDate, EndDate, TotalParticipants, TotalTrades, TotalVolume, PrizePool, Winners, Statistics, TopSymbols, GeneratedAt'),
    ('prize_won', 'Prize winning notification', 'UserName, ContestID, ContestName, FinalRank, TotalParticipants, PrizeAmount, FinalPnL, TralentScoreGain, ResultsURL'),
    ('daily_digest', 'Daily platform digest', 'Date, TotalAlerts, CriticalCount, ResolvedCount, Services, Alerts, TopErrors, GeneratedAt'),
    ('bug_report', 'Bug report email', 'Title, Message, Severity, SeverityColor, Service, Timestamp, TraceID, SpanID, StackTrace, Metadata')
ON CONFLICT (slug) DO NOTHING;

-- ============================================================================
-- COMMENTS FOR DOCUMENTATION
-- ============================================================================

COMMENT ON TABLE email_templates IS 'Custom email template overrides for admin customization';
COMMENT ON COLUMN email_templates.slug IS 'Template identifier matching the embedded template name';
COMMENT ON COLUMN email_templates.subject IS 'Optional custom subject line for the email';
COMMENT ON COLUMN email_templates.html_content IS 'Custom HTML template content. NULL or empty means use embedded default.';
COMMENT ON COLUMN email_templates.description IS 'Human-readable description of when this template is used';
COMMENT ON COLUMN email_templates.variables IS 'Comma-separated list of Go template variables available in this template';
COMMENT ON COLUMN email_templates.updated_by IS 'Last admin user who modified this template';
