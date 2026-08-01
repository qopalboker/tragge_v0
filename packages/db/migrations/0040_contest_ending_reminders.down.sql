-- 0040_contest_ending_reminders.down.sql
-- Remove the contest_ending email template

DELETE FROM email_templates WHERE slug = 'contest_ending';
