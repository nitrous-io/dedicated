-- drop_type_from_plans
-- Version: 1456903956

ALTER TABLE plans DROP COLUMN type;
DROP TYPE plan_type;
