-- add_trial_eligible_to_billing_profiles
-- Version: 1445503882

ALTER TABLE billing_profiles ADD COLUMN trial_eligible bool DEFAULT true;
