-- add_trial_period_to_plans
-- Version: 1442826252

ALTER TABLE plans ADD COLUMN trial_period_days bigint DEFAULT 0 NOT NULL;
