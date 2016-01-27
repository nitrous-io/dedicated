-- add_usage_notices_sent
-- Version: 1453779095

ALTER TABLE users ADD COLUMN usage_notices_sent bigint DEFAULT 0 NOT NULL;
