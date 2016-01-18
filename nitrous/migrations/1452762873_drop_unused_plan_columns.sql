-- drop_unused_plan_columns
-- Version: 1452762873

ALTER TABLE plans DROP CONSTRAINT hosts_paid_plan_id_fkey;
ALTER TABLE plans DROP COLUMN paid_plan_id;

ALTER TABLE plans
  DROP COLUMN base_plan_id,
  DROP COLUMN clean_preview,
  DROP COLUMN desktop_app,
  DROP COLUMN ssh,
  DROP COLUMN custom_domain,
  DROP COLUMN custom_templates;
