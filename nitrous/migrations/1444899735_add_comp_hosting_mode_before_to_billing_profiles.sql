-- add_comp_hosting_mode_before_to_billing_profiles
-- Version: 1444899735

ALTER TABLE billing_profiles ADD COLUMN comp_hosting_mode_before timestamp without time zone DEFAULT NULL;
