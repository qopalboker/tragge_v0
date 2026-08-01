DROP TRIGGER IF EXISTS trg_check_max_template_versions ON email_template_versions;
DROP FUNCTION IF EXISTS check_max_template_versions();
DROP TABLE IF EXISTS email_template_versions;
