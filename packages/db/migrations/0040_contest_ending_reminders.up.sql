-- 0040_contest_ending_reminders.up.sql
-- Register the contest_ending email template for end-of-contest reminders

INSERT INTO email_templates (slug, description, variables) VALUES
    ('contest_ending', 'Contest ending reminder sent to participants before a running contest ends', 'ContestID, ContestName, EndTime, TimeUntilEnd, Duration, StartingBalance, ParticipantCount, Symbols, TradingURL')
ON CONFLICT (slug) DO NOTHING;
