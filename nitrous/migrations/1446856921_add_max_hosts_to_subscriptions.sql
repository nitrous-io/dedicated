-- add_max_hosts_to_subscriptions
-- Version: 1446856921

ALTER TABLE subscriptions ADD COLUMN max_hosts bigint NOT NULL DEFAULT 2;
