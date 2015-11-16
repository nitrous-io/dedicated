-- AddMultiHostSupport
-- Version: 1447700865

ALTER TABLE plans ADD COLUMN max_hosts bigint DEFAULT NULL;
ALTER TABLE subscriptions ADD COLUMN multi_host bool DEFAULT FALSE;
