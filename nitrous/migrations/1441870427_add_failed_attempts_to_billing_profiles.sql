-- add_failed_attempts_to_billing_profiles
-- Version: 1441870427
-- Please specify your SQL migration here

ALTER TABLE billing_profiles ADD COLUMN failed_attempts int NOT NULL DEFAULT 0;
