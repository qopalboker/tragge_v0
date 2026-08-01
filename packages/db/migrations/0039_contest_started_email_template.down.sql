-- 0039_contest_started_email_template.down.sql
-- Remove the contest_started email template

DELETE FROM email_templates WHERE slug = 'contest_started';
