-- remove_hotswap_columns
-- Version: 1452648884

CREATE TABLE backup_provider_templates AS (SELECT * FROM provider_templates);

-- DELETE FROM provider_templates WHERE hotswap = false;
-- This failed to run on production because of FK constraints on
-- `hosts.provider_template_id` column.

ALTER TABLE hosts DROP COLUMN hotswap;
ALTER TABLE provider_templates DROP COLUMN hotswap;
