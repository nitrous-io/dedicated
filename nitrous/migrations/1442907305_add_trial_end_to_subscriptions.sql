-- add_trial_end_to_subscriptions
-- Version: 1442907305

ALTER TABLE subscriptions ADD COLUMN trial_end timestamp without time zone DEFAULT NULL;
