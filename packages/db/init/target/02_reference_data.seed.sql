-- FND-004 intentionally seeds no domain rows.
--
-- DATA-005 will introduce the persistent System Participant account.
-- SCH-003 will create its Free Practice Contest registrations.
-- Symbol, template, rank-band, and other reference records remain planned and
-- must be introduced by their owning roadmap tasks with stable identifiers.

SELECT 'fnd004_no_domain_seed_data' AS seed_status;
