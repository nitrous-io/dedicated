-- add_max_hosts_to_plans
-- Version: 1446852924

ALTER TABLE plans ADD COLUMN max_hosts bigint NOT NULL DEFAULT 1;
