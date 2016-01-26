-- add_usage_quota_restored_at
-- Version: 1453444021

ALTER TABLE users ADD COLUMN usage_quota_restored_at timestamp without time zone;
