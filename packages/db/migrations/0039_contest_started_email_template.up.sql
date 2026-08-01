-- 0039_contest_started_email_template.up.sql
-- Register the contest_started email template

INSERT INTO email_templates (slug, description, variables) VALUES
    ('contest_started', 'Contest started notification sent to all participants when a contest goes live', 'ContestName, ContestID, TradeURL, EndsAt')
ON CONFLICT (slug) DO NOTHING;
