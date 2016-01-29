-- rename_usage_quota_fields
-- Version: 1453349174

-- usage_quota: time (in seconds) that a user is allowed to use a free host.
ALTER TABLE users RENAME COLUMN max_running_time TO usage_quota;
ALTER TABLE users ALTER COLUMN usage_quota SET DEFAULT 180000; -- 50 hours

-- usage_quota_period: time (in seconds) after which the usage_quota is reset.
ALTER TABLE users RENAME COLUMN refresh_period TO usage_quota_period;
ALTER TABLE users ALTER COLUMN usage_quota_period SET DEFAULT 2592000; -- 30 days

ALTER TABLE users ADD COLUMN usage_remaining bigint DEFAULT 180000; -- 50 hours

